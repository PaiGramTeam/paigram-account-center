package usecase

import (
	"context"
	"errors"
	"time"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

const authKeyArtifactType = "authkey"
const maximumAuthKeyTTL = 5 * time.Minute
const minimumAuthKeyReturnTTL = 5 * time.Second

var ErrCredentialRequiresAttention = errors.New("credential requires attention")

type GetAuthKeyOutput struct {
	AuthKey   string
	ExpiresAt time.Time
}

type AuthkeyUsecase struct {
	credentialRepo biz.CredentialRepository
	artifactRepo   biz.ArtifactRepository
	artifacts      *ArtifactLifecycle
	client         platformmihomo.Client
	encryptionKey  []byte
}

func NewAuthkeyUsecase(
	credentialRepo biz.CredentialRepository,
	artifactRepo biz.ArtifactRepository,
	artifacts *ArtifactLifecycle,
	client platformmihomo.Client,
	encryptionKey []byte,
) *AuthkeyUsecase {
	return &AuthkeyUsecase{
		credentialRepo: credentialRepo,
		artifactRepo:   artifactRepo,
		artifacts:      artifacts,
		client:         client,
		encryptionKey:  encryptionKey,
	}
}

func (uc *AuthkeyUsecase) GetAuthKey(ctx context.Context, accountKey string, playerID string) (*GetAuthKeyOutput, error) {
	credential, err := uc.credentialRepo.GetByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrCredentialNotFound
	}
	var output *GetAuthKeyOutput
	var revocationIntent *biz.ArtifactRevocationIntent
	var operationErr error
	err = uc.runInTransaction(ctx, func(txCtx context.Context) error {
		current, err := uc.credentialRepo.GetByBindingRefForUpdate(txCtx, credential.BindingRef)
		if err != nil {
			return err
		}
		if current == nil || current.AccountKey != accountKey {
			return ErrCredentialNotFound
		}
		output, revocationIntent, operationErr = uc.getAuthKeyLocked(txCtx, current, playerID)
		if isCredentialAttentionError(operationErr) {
			current.Status = classifyCredentialStatus(operationErr)
			if err := uc.credentialRepo.Save(txCtx, current); err != nil {
				return err
			}
			if err := uc.artifacts.InvalidateBinding(txCtx, current.BindingRef); err != nil {
				operationErr = errors.Join(operationErr, err)
			}
			return nil
		}
		return operationErr
	})
	if err != nil {
		return nil, errors.Join(err, uc.artifacts.RevokeIssuedArtifact(ctx, revocationIntent))
	}
	if operationErr != nil {
		return nil, errors.Join(operationErr, uc.artifacts.RevokeIssuedArtifact(ctx, revocationIntent))
	}
	if revocationIntent != nil {
		if err := uc.artifacts.FinalizeIssuedArtifact(ctx, revocationIntent.IntentID); err != nil {
			return nil, errors.Join(err, uc.artifacts.RevokeIssuedArtifact(ctx, revocationIntent))
		}
	}
	_, _ = uc.artifactRepo.GetByBindingRef(ctx, credential.BindingRef, authKeyArtifactType, playerID)
	return output, nil
}

func (uc *AuthkeyUsecase) getAuthKeyLocked(ctx context.Context, credential *biz.Credential, playerID string) (*GetAuthKeyOutput, *biz.ArtifactRevocationIntent, error) {
	if credential.Status != "active" || (credential.ExpiresAt != nil && !credential.ExpiresAt.After(time.Now())) {
		if err := uc.artifacts.InvalidateBinding(ctx, credential.BindingRef); err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrCredentialRequiresAttention
	}

	cookieBundleJSON, err := internalcrypto.DecryptString(uc.encryptionKey, credential.CredentialBlob)
	if err != nil {
		return nil, nil, err
	}
	pending, err := uc.artifactRepo.HasRevocationPending(ctx, credential.BindingRef)
	if err != nil {
		return nil, nil, err
	}
	if pending {
		return nil, nil, biz.ErrArtifactRevocationPending
	}

	artifact, err := uc.artifactRepo.GetByBindingRef(ctx, credential.BindingRef, authKeyArtifactType, playerID)
	if err != nil {
		return nil, nil, err
	}
	if artifact != nil {
		if time.Until(artifact.ExpiresAt) < minimumAuthKeyReturnTTL {
			if err := uc.artifacts.InvalidateBinding(ctx, credential.BindingRef); err != nil {
				return nil, nil, err
			}
			artifact = nil
		}
	}
	if artifact != nil {
		authKey, err := internalcrypto.DecryptArtifact(uc.encryptionKey, artifact.ArtifactValue, artifact.BindingRef, artifact.AccountKey, artifact.ArtifactType, artifact.ScopeKey)
		if err != nil {
			return nil, nil, err
		}
		return &GetAuthKeyOutput{AuthKey: authKey, ExpiresAt: artifact.ExpiresAt}, nil, nil
	}

	issuer, ok := uc.client.(platformmihomo.BoundedAuthKeyIssuer)
	if !ok {
		return nil, nil, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorUnavailable}
	}
	_, ok = uc.client.(platformmihomo.AuthKeyRevoker)
	if !ok {
		return nil, nil, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorUnavailable}
	}
	authKey, expiresInSeconds, err := issuer.IssueAuthKeyWithTTL(ctx, cookieBundleJSON, playerID, maximumAuthKeyTTL)
	if err != nil {
		return nil, nil, err
	}
	if authKey == "" {
		return nil, nil, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorInvalidResponse}
	}

	encryptedAuthKey, err := internalcrypto.EncryptArtifact(uc.encryptionKey, authKey, credential.BindingRef, credential.AccountKey, authKeyArtifactType, playerID)
	if err != nil {
		return nil, nil, errors.Join(err, uc.artifacts.RevokeUntrackedAuthKey(ctx, authKey))
	}
	expiresAt := time.Now().UTC().Add(maximumAuthKeyTTL)
	validTTL := expiresInSeconds >= int64(minimumAuthKeyReturnTTL/time.Second) && expiresInSeconds <= int64(maximumAuthKeyTTL/time.Second)
	if validTTL {
		expiresAt = time.Now().UTC().Add(time.Duration(expiresInSeconds) * time.Second)
	}
	artifact = &biz.Artifact{
		BindingRef:    credential.BindingRef,
		AccountKey:    credential.AccountKey,
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: encryptedAuthKey,
		ScopeKey:      playerID,
		ExpiresAt:     expiresAt,
	}
	revocationIntent, err := uc.artifacts.StageIssuedArtifact(ctx, artifact)
	if err != nil {
		return nil, nil, errors.Join(err, uc.artifacts.RevokeUntrackedAuthKey(ctx, authKey))
	}
	if !validTTL {
		return nil, revocationIntent, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorInvalidResponse}
	}
	if err := uc.artifactRepo.PutIfCredentialCurrent(ctx, artifact, credential.Generation); err != nil {
		return nil, revocationIntent, err
	}

	return &GetAuthKeyOutput{AuthKey: authKey, ExpiresAt: expiresAt}, revocationIntent, nil
}

func (uc *AuthkeyUsecase) InvalidateBinding(ctx context.Context, bindingRef string) error {
	return uc.artifacts.InvalidateBinding(ctx, bindingRef)
}

func (uc *AuthkeyUsecase) ConfirmDeliverable(ctx context.Context, bindingRef string, authorize func(context.Context) error) error {
	return uc.runInTransaction(ctx, func(txCtx context.Context) error {
		credential, err := uc.credentialRepo.GetByBindingRefForUpdate(txCtx, bindingRef)
		if err != nil {
			return err
		}
		if credential == nil {
			return ErrCredentialNotFound
		}
		if authorize != nil {
			if err := authorize(txCtx); err != nil {
				return err
			}
		}
		if credential.Status != "active" || (credential.ExpiresAt != nil && !credential.ExpiresAt.After(time.Now())) {
			return ErrCredentialRequiresAttention
		}
		pending, err := uc.artifactRepo.HasRevocationPending(txCtx, bindingRef)
		if err != nil {
			return err
		}
		if pending {
			return biz.ErrArtifactRevocationPending
		}
		return nil
	})
}

func (uc *AuthkeyUsecase) runInTransaction(ctx context.Context, fn func(context.Context) error) error {
	if transactioner, ok := uc.credentialRepo.(credentialTransactioner); ok {
		return transactioner.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}
