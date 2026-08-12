package usecase

import (
	"context"
	"errors"
	"time"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

var ErrCredentialNotFound = errors.New("credential not found")

type GetCredentialStatusOutput struct {
	Status          CredentialStatus
	LastValidatedAt *time.Time
}

type ValidateCredentialOutput struct {
	Status    CredentialStatus
	ErrorCode string
}

type RefreshCredentialOutput struct {
	Status      CredentialStatus
	RefreshedAt *time.Time
}

type StatusUsecase struct {
	credentials   biz.CredentialRepository
	hoyoClient    platformmihomo.Client
	encryptionKey []byte
}

func NewStatusUsecase(
	credentials biz.CredentialRepository,
	hoyoClient platformmihomo.Client,
	encryptionKey []byte,
) *StatusUsecase {
	return &StatusUsecase{
		credentials:   credentials,
		hoyoClient:    hoyoClient,
		encryptionKey: encryptionKey,
	}
}

func (uc *StatusUsecase) GetCredentialStatus(ctx context.Context, platformAccountID string) (*GetCredentialStatusOutput, error) {
	credential, err := uc.getCredential(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}

	return &GetCredentialStatusOutput{
		Status:          credentialStatusFromStorage(credential.Status),
		LastValidatedAt: credential.LastValidatedAt,
	}, nil
}

func (uc *StatusUsecase) ValidateCredential(ctx context.Context, platformAccountID string) (*ValidateCredentialOutput, error) {
	credential, err := uc.getCredential(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}

	status, errCode, err := uc.revalidate(ctx, credential, false)
	if err != nil {
		return nil, err
	}

	return &ValidateCredentialOutput{Status: status, ErrorCode: errCode}, nil
}

// RefreshCredential performs version 1 credential revalidation.
// It does not rotate or replace credential material; it validates the stored
// credential blob, updates status metadata, and records LastRefreshedAt when
// validation succeeds.
func (uc *StatusUsecase) RefreshCredential(ctx context.Context, platformAccountID string) (*RefreshCredentialOutput, error) {
	credential, err := uc.getCredential(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}

	status, _, err := uc.revalidate(ctx, credential, true)
	if err != nil {
		return nil, err
	}

	return &RefreshCredentialOutput{Status: status, RefreshedAt: credential.LastRefreshedAt}, nil
}

func (uc *StatusUsecase) getCredential(ctx context.Context, platformAccountID string) (*biz.Credential, error) {
	credential, err := uc.credentials.GetByPlatformAccountID(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrCredentialNotFound
	}
	return credential, nil
}

func (uc *StatusUsecase) revalidate(ctx context.Context, credential *biz.Credential, markRefreshed bool) (CredentialStatus, string, error) {
	cookieBundleJSON, err := internalcrypto.DecryptString(uc.encryptionKey, credential.CredentialBlob)
	if err != nil {
		return CredentialStatusUnspecified, "decrypt_failed", err
	}

	_, _, _, err = uc.hoyoClient.ValidateAndDiscover(ctx, cookieBundleJSON, credential.Region)
	now := time.Now().UTC()
	credential.LastValidatedAt = &now

	if err != nil {
		credential.Status = classifyCredentialStatus(err)
		if saveErr := uc.credentials.Save(ctx, credential); saveErr != nil {
			return CredentialStatusUnspecified, "save_failed", saveErr
		}
		return credentialStatusFromStorage(credential.Status), classifyErrorCode(err), nil
	}

	credential.Status = "active"
	if markRefreshed {
		credential.LastRefreshedAt = &now
	}
	if saveErr := uc.credentials.Save(ctx, credential); saveErr != nil {
		return CredentialStatusUnspecified, "save_failed", saveErr
	}

	return CredentialStatusActive, "", nil
}

func classifyCredentialStatus(err error) string {
	switch {
	case platformmihomo.IsErrorKind(err, platformmihomo.ErrorChallengeRequired):
		return "challenge_required"
	case platformmihomo.IsErrorKind(err, platformmihomo.ErrorExpiredCredential):
		return "expired"
	default:
		return "invalid"
	}
}

func classifyErrorCode(err error) string {
	switch {
	case platformmihomo.IsErrorKind(err, platformmihomo.ErrorChallengeRequired):
		return "CHALLENGE_REQUIRED"
	case platformmihomo.IsErrorKind(err, platformmihomo.ErrorExpiredCredential):
		return "CREDENTIAL_EXPIRED"
	default:
		return "INVALID_CREDENTIAL"
	}
}
