package usecase

import (
	"context"
	"time"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

const authKeyArtifactType = "authkey"

type GetAuthKeyOutput struct {
	AuthKey   string
	ExpiresAt time.Time
}

type AuthkeyUsecase struct {
	credentialRepo biz.CredentialRepository
	artifactRepo   biz.ArtifactRepository
	client         platformmihomo.Client
	encryptionKey  []byte
}

func NewAuthkeyUsecase(
	credentialRepo biz.CredentialRepository,
	artifactRepo biz.ArtifactRepository,
	client platformmihomo.Client,
	encryptionKey []byte,
) *AuthkeyUsecase {
	return &AuthkeyUsecase{
		credentialRepo: credentialRepo,
		artifactRepo:   artifactRepo,
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

	cookieBundleJSON, err := internalcrypto.DecryptString(uc.encryptionKey, credential.CredentialBlob)
	if err != nil {
		return nil, err
	}

	artifact, err := uc.artifactRepo.GetByBindingRef(ctx, credential.BindingRef, authKeyArtifactType, playerID)
	if err != nil {
		return nil, err
	}
	if artifact != nil {
		authKey, err := internalcrypto.DecryptArtifact(uc.encryptionKey, artifact.ArtifactValue, artifact.BindingRef, artifact.AccountKey, artifact.ArtifactType, artifact.ScopeKey)
		if err != nil {
			return nil, err
		}
		return &GetAuthKeyOutput{AuthKey: authKey, ExpiresAt: artifact.ExpiresAt}, nil
	}

	authKey, expiresInSeconds, err := uc.client.IssueAuthKey(ctx, cookieBundleJSON, playerID)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(time.Duration(expiresInSeconds) * time.Second)
	encryptedAuthKey, err := internalcrypto.EncryptArtifact(uc.encryptionKey, authKey, credential.BindingRef, accountKey, authKeyArtifactType, playerID)
	if err != nil {
		return nil, err
	}
	artifact = &biz.Artifact{
		BindingRef:    credential.BindingRef,
		AccountKey:    accountKey,
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: encryptedAuthKey,
		ScopeKey:      playerID,
		ExpiresAt:     expiresAt,
	}
	if err := uc.artifactRepo.Put(ctx, artifact); err != nil {
		return nil, err
	}

	return &GetAuthKeyOutput{AuthKey: authKey, ExpiresAt: expiresAt}, nil
}
