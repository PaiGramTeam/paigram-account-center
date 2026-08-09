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

func (uc *AuthkeyUsecase) GetAuthKey(ctx context.Context, platformAccountID string, playerID string) (*GetAuthKeyOutput, error) {
	credential, err := uc.credentialRepo.GetByPlatformAccountID(ctx, platformAccountID)
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

	artifact, err := uc.artifactRepo.GetByBindingID(ctx, credential.BindingID, authKeyArtifactType, playerID)
	if err != nil {
		return nil, err
	}
	if artifact != nil {
		return &GetAuthKeyOutput{AuthKey: artifact.ArtifactValue, ExpiresAt: artifact.ExpiresAt}, nil
	}

	authKey, expiresInSeconds, err := uc.client.IssueAuthKey(ctx, cookieBundleJSON, playerID)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(time.Duration(expiresInSeconds) * time.Second)
	artifact = &biz.Artifact{
		BindingID:         credential.BindingID,
		PlatformAccountID: platformAccountID,
		ArtifactType:      authKeyArtifactType,
		ArtifactValue:     authKey,
		ScopeKey:          playerID,
		ExpiresAt:         expiresAt,
	}
	if err := uc.artifactRepo.Put(ctx, artifact); err != nil {
		return nil, err
	}

	return &GetAuthKeyOutput{AuthKey: authKey, ExpiresAt: expiresAt}, nil
}
