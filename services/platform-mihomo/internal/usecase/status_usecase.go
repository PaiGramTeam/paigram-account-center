package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

var ErrCredentialNotFound = errors.New("credential not found")

type GetCredentialStatusOutput struct {
	Status          v1.CredentialStatus
	LastValidatedAt *time.Time
}

type ValidateCredentialOutput struct {
	Status    v1.CredentialStatus
	ErrorCode string
}

type RefreshCredentialOutput struct {
	Status      v1.CredentialStatus
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
		Status:          toCredentialStatus(credential.Status),
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

func (uc *StatusUsecase) revalidate(ctx context.Context, credential *biz.Credential, markRefreshed bool) (v1.CredentialStatus, string, error) {
	cookieBundleJSON, err := internalcrypto.DecryptString(uc.encryptionKey, credential.CredentialBlob)
	if err != nil {
		return v1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED, "decrypt_failed", err
	}

	_, _, _, err = uc.hoyoClient.ValidateAndDiscover(ctx, cookieBundleJSON, credential.Region)
	now := time.Now().UTC()
	credential.LastValidatedAt = &now

	if err != nil {
		credential.Status = classifyCredentialStatus(err)
		if saveErr := uc.credentials.Save(ctx, credential); saveErr != nil {
			return v1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED, "save_failed", saveErr
		}
		return toCredentialStatus(credential.Status), classifyErrorCode(err), nil
	}

	credential.Status = "active"
	if markRefreshed {
		credential.LastRefreshedAt = &now
	}
	if saveErr := uc.credentials.Save(ctx, credential); saveErr != nil {
		return v1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED, "save_failed", saveErr
	}

	return v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, "", nil
}

func toCredentialStatus(status string) v1.CredentialStatus {
	switch status {
	case "active":
		return v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE
	case "expired":
		return v1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED
	case "invalid":
		return v1.CredentialStatus_CREDENTIAL_STATUS_INVALID
	case "challenge_required":
		return v1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED
	default:
		return v1.CredentialStatus_CREDENTIAL_STATUS_UNSPECIFIED
	}
}

func classifyCredentialStatus(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "challenge"):
		return "challenge_required"
	case strings.Contains(message, "expire"):
		return "expired"
	default:
		return "invalid"
	}
}

func classifyErrorCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "challenge"):
		return "CHALLENGE_REQUIRED"
	case strings.Contains(message, "expire"):
		return "CREDENTIAL_EXPIRED"
	default:
		return "INVALID_CREDENTIAL"
	}
}
