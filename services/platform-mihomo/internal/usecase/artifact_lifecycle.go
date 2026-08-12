package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

var ErrArtifactStorageNotConfigured = errors.New("artifact storage is not configured")

const artifactCompensationTimeout = 5 * time.Second
const artifactProvisionalRecoveryDelay = 30 * time.Second
const artifactRevocationLease = 30 * time.Second
const artifactIntentProvisional = "provisional"
const artifactIntentReady = "ready"

type ArtifactLifecycle struct {
	repository biz.ArtifactRepository
	revoker    platformmihomo.AuthKeyRevoker
	key        internalcrypto.KeyProvider
}

type ArtifactLifecycleConfig struct {
	Revoker       platformmihomo.AuthKeyRevoker
	EncryptionKey internalcrypto.KeyProvider
}

func NewArtifactLifecycle(repository biz.ArtifactRepository, configs ...ArtifactLifecycleConfig) *ArtifactLifecycle {
	lifecycle := &ArtifactLifecycle{repository: repository}
	if len(configs) > 0 {
		lifecycle.revoker = configs[0].Revoker
		lifecycle.key = configs[0].EncryptionKey
	}
	return lifecycle
}

func (l *ArtifactLifecycle) InvalidateBinding(ctx context.Context, bindingRef string) error {
	if l == nil || l.repository == nil {
		return ErrArtifactStorageNotConfigured
	}
	if bindingRef == "" {
		return nil
	}
	artifacts, err := l.repository.ListByBindingRef(ctx, bindingRef)
	if err != nil {
		return err
	}
	intents := make([]*biz.ArtifactRevocationIntent, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.ArtifactType != authKeyArtifactType {
			continue
		}
		intent, err := l.stageArtifact(ctx, artifact, artifactIntentReady)
		if err != nil {
			return err
		}
		intents = append(intents, intent)
	}
	compensationCtx, cancel := artifactCompensationContext(ctx)
	defer cancel()
	if err := l.repository.MarkRevocationPendingImmediately(compensationCtx, bindingRef); err != nil {
		if len(intents) > 0 {
			return errors.Join(biz.ErrArtifactRevocationPending, err)
		}
		return err
	}
	var revokeErr error
	for _, intent := range intents {
		if err := l.RevokeIssuedArtifact(compensationCtx, intent); err != nil {
			revokeErr = errors.Join(revokeErr, err)
		}
	}
	if revokeErr != nil {
		return errors.Join(biz.ErrArtifactRevocationPending, revokeErr)
	}
	return l.repository.DeleteByBindingRefImmediately(compensationCtx, bindingRef)
}

func (l *ArtifactLifecycle) StageIssuedArtifact(ctx context.Context, artifact *biz.Artifact) (*biz.ArtifactRevocationIntent, error) {
	return l.stageArtifact(ctx, artifact, artifactIntentProvisional)
}

func (l *ArtifactLifecycle) stageArtifact(ctx context.Context, artifact *biz.Artifact, state string) (*biz.ArtifactRevocationIntent, error) {
	if l == nil || l.repository == nil {
		return nil, ErrArtifactStorageNotConfigured
	}
	intentID, err := newArtifactIntentID()
	if err != nil {
		return nil, err
	}
	readyAfter := time.Now().UTC()
	if state == artifactIntentProvisional {
		readyAfter = readyAfter.Add(artifactProvisionalRecoveryDelay)
	}
	intent := &biz.ArtifactRevocationIntent{
		IntentID:      intentID,
		BindingRef:    artifact.BindingRef,
		AccountKey:    artifact.AccountKey,
		ArtifactType:  artifact.ArtifactType,
		ArtifactValue: artifact.ArtifactValue,
		ScopeKey:      artifact.ScopeKey,
		ExpiresAt:     artifact.ExpiresAt,
		State:         state,
		ReadyAfter:    readyAfter,
	}
	compensationCtx, cancel := artifactCompensationContext(ctx)
	defer cancel()
	persisted, err := l.repository.PutRevocationIntentImmediately(compensationCtx, intent)
	if err != nil {
		return nil, err
	}
	return persisted, nil
}

func (l *ArtifactLifecycle) FinalizeIssuedArtifact(ctx context.Context, intentID string) error {
	if l == nil || l.repository == nil {
		return ErrArtifactStorageNotConfigured
	}
	compensationCtx, cancel := artifactCompensationContext(ctx)
	defer cancel()
	return l.repository.FinalizeRevocationIntentImmediately(compensationCtx, intentID)
}

func (l *ArtifactLifecycle) RevokeIssuedArtifact(ctx context.Context, intent *biz.ArtifactRevocationIntent) error {
	return l.revokeIssuedArtifact(ctx, intent, true)
}

func (l *ArtifactLifecycle) revokeIssuedArtifact(ctx context.Context, intent *biz.ArtifactRevocationIntent, markReady bool) error {
	if l == nil || l.repository == nil || l.revoker == nil {
		return ErrArtifactStorageNotConfigured
	}
	if intent == nil {
		return nil
	}
	compensationCtx, cancel := artifactCompensationContext(ctx)
	defer cancel()
	if markReady {
		if err := l.repository.MarkRevocationIntentReadyImmediately(compensationCtx, intent.IntentID); err != nil {
			return errors.Join(biz.ErrArtifactRevocationPending, err)
		}
	}
	authKey, err := internalcrypto.DecryptArtifact(
		l.key,
		intent.ArtifactValue,
		intent.BindingRef,
		intent.AccountKey,
		intent.ArtifactType,
		intent.ScopeKey,
	)
	if err != nil {
		return err
	}
	if err := l.revoker.RevokeAuthKey(compensationCtx, authKey); err != nil {
		return errors.Join(biz.ErrArtifactRevocationPending, err)
	}
	if err := l.repository.DeleteArtifactImmediately(
		compensationCtx,
		intent.BindingRef,
		intent.ArtifactType,
		intent.ScopeKey,
		intent.ArtifactValue,
	); err != nil {
		return errors.Join(biz.ErrArtifactRevocationPending, err)
	}
	return l.repository.DeleteRevocationIntentImmediately(compensationCtx, intent.IntentID)
}

func (l *ArtifactLifecycle) RevokeUntrackedAuthKey(ctx context.Context, authKey string) error {
	if l == nil || l.revoker == nil {
		return ErrArtifactStorageNotConfigured
	}
	compensationCtx, cancel := artifactCompensationContext(ctx)
	defer cancel()
	return l.revoker.RevokeAuthKey(compensationCtx, authKey)
}

func (l *ArtifactLifecycle) DeleteExpired(ctx context.Context, expiredBefore time.Time) (int64, error) {
	if l == nil || l.repository == nil {
		return 0, ErrArtifactStorageNotConfigured
	}
	return l.repository.DeleteExpired(ctx, expiredBefore)
}

func (l *ArtifactLifecycle) RetryPending(ctx context.Context) error {
	if l == nil || l.repository == nil {
		return ErrArtifactStorageNotConfigured
	}
	now := time.Now().UTC()
	leaseToken, err := newArtifactIntentID()
	if err != nil {
		return err
	}
	intents, err := l.repository.ClaimRevocationIntents(ctx, now, now.Add(artifactRevocationLease), leaseToken)
	if err != nil {
		return err
	}
	var retryErr error
	for _, intent := range intents {
		if intent.State == artifactIntentProvisional {
			shouldRevoke, err := l.repository.ResolveProvisionalRevocationIntent(ctx, intent.IntentID, leaseToken)
			if err != nil {
				retryErr = errors.Join(retryErr, err)
				_ = l.repository.ReleaseRevocationIntentClaim(ctx, intent.IntentID, leaseToken)
				continue
			}
			if !shouldRevoke {
				continue
			}
		}
		if err := l.revokeIssuedArtifact(ctx, intent, false); err != nil {
			retryErr = errors.Join(retryErr, err)
			_ = l.repository.ReleaseRevocationIntentClaim(ctx, intent.IntentID, leaseToken)
		}
	}
	return retryErr
}

func artifactCompensationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), artifactCompensationTimeout)
}

func newArtifactIntentID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}
