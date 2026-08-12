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
	Status                  CredentialStatus
	RefreshedAt             *time.Time
	Profiles                []*ProfileSummary
	ProfileSnapshotComplete bool
	ProfileRevision         uint64
	ProfileObservedRevision uint64
}

type StatusUsecase struct {
	credentials   biz.CredentialRepository
	profiles      biz.ProfileRepository
	hoyoClient    platformmihomo.Client
	encryptionKey []byte
}

func NewStatusUsecase(
	credentials biz.CredentialRepository,
	profiles biz.ProfileRepository,
	hoyoClient platformmihomo.Client,
	encryptionKey []byte,
) *StatusUsecase {
	return &StatusUsecase{
		credentials:   credentials,
		profiles:      profiles,
		hoyoClient:    hoyoClient,
		encryptionKey: encryptionKey,
	}
}

func (uc *StatusUsecase) GetCredentialStatus(ctx context.Context, accountKey string) (*GetCredentialStatusOutput, error) {
	credential, err := uc.getCredential(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	return &GetCredentialStatusOutput{
		Status:          credentialStatusFromStorage(credential.Status),
		LastValidatedAt: credential.LastValidatedAt,
	}, nil
}

func (uc *StatusUsecase) ValidateCredential(ctx context.Context, accountKey string) (*ValidateCredentialOutput, error) {
	credential, err := uc.getCredential(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	status, errCode, err := uc.revalidate(ctx, credential, false)
	if err != nil {
		return nil, err
	}

	return &ValidateCredentialOutput{Status: status, ErrorCode: errCode}, nil
}

func (uc *StatusUsecase) RefreshCredential(ctx context.Context, accountKey string) (*RefreshCredentialOutput, error) {
	credential, err := uc.getCredential(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	cookieBundleJSON, err := internalcrypto.DecryptString(uc.encryptionKey, credential.CredentialBlob)
	if err != nil {
		return nil, err
	}
	existingProfiles, err := uc.profiles.ListByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	refresher, ok := uc.hoyoClient.(platformmihomo.CredentialRefresher)
	if !ok {
		return nil, &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorUnavailable}
	}
	refreshResult, validationErr := refresher.RefreshCredential(ctx, cookieBundleJSON, credential.Region)
	now := time.Now().UTC()
	credential.LastValidatedAt = &now
	if validationErr != nil {
		if !isCredentialAttentionError(validationErr) {
			return nil, validationErr
		}
		credential.Status = classifyCredentialStatus(validationErr)
		credential.ProfileSnapshotComplete = false
		if err := uc.credentials.Save(ctx, credential); err != nil {
			return nil, err
		}
		return &RefreshCredentialOutput{
			Status:                  credentialStatusFromStorage(credential.Status),
			Profiles:                profileSummariesFromProfiles(existingProfiles),
			ProfileSnapshotComplete: false,
			ProfileRevision:         credential.ProfileRevision,
			ProfileObservedRevision: credential.ProfileObservedRevision,
		}, nil
	}
	if err := platformmihomo.ValidateRefreshResult(refreshResult, now); err != nil {
		return nil, err
	}
	if credential.AccountID != "" && credential.AccountID != refreshResult.AccountID {
		return nil, errors.New("discovered account does not match credential")
	}
	rotatedBlob, err := internalcrypto.EncryptString(uc.encryptionKey, refreshResult.CredentialBundleJSON)
	if err != nil {
		return nil, err
	}

	previousPrimary := primaryProfileIdentity(existingProfiles)
	if err := uc.profiles.DeleteByAccountKey(ctx, accountKey); err != nil {
		return nil, err
	}
	refreshedProfiles := make([]*ProfileSummary, 0, len(refreshResult.Profiles))
	primaryIndex := refreshedPrimaryIndex(refreshResult.Profiles, previousPrimary)
	for index, discovered := range refreshResult.Profiles {
		profile := &biz.Profile{
			BindingRef:   credential.BindingRef,
			AccountKey:   accountKey,
			ProfileRef:   FormatProfileRef(accountKey, discovered.GameBiz, discovered.Region, discovered.PlayerID),
			GameBiz:      discovered.GameBiz,
			Region:       discovered.Region,
			PlayerID:     discovered.PlayerID,
			Nickname:     discovered.Nickname,
			Level:        int(discovered.Level),
			IsDefault:    index == primaryIndex,
			DiscoveredAt: now,
		}
		if err := uc.profiles.Save(ctx, profile); err != nil {
			return nil, err
		}
		refreshedProfiles = append(refreshedProfiles, toProfileSummary(profile))
	}

	credential.AccountID = refreshResult.AccountID
	credential.Region = refreshResult.Region
	credential.CredentialBlob = rotatedBlob
	credential.Status = "active"
	credential.LastRefreshedAt = &now
	expiresAt := refreshResult.ExpiresAt.UTC()
	credential.ExpiresAt = &expiresAt
	credential.ProfileSnapshotComplete = true
	credential.ProfileRevision = nextProfileRevision(credential.ProfileRevision, credential.ProfileObservedRevision)
	credential.ProfileObservedRevision = credential.ProfileRevision
	if err := uc.credentials.Save(ctx, credential); err != nil {
		return nil, err
	}

	return &RefreshCredentialOutput{
		Status:                  CredentialStatusActive,
		RefreshedAt:             credential.LastRefreshedAt,
		Profiles:                refreshedProfiles,
		ProfileSnapshotComplete: true,
		ProfileRevision:         credential.ProfileRevision,
		ProfileObservedRevision: credential.ProfileObservedRevision,
	}, nil
}

func nextProfileRevision(revision, observedRevision uint64) uint64 {
	if observedRevision > revision {
		return observedRevision + 1
	}
	return revision + 1
}

func isCredentialAttentionError(err error) bool {
	return platformmihomo.IsErrorKind(err, platformmihomo.ErrorInvalidCredential) ||
		platformmihomo.IsErrorKind(err, platformmihomo.ErrorExpiredCredential) ||
		platformmihomo.IsErrorKind(err, platformmihomo.ErrorChallengeRequired)
}

func primaryProfileIdentity(profiles []*biz.Profile) *biz.ProfileIdentity {
	for _, profile := range profiles {
		if profile.IsDefault {
			return &biz.ProfileIdentity{PlayerID: profile.PlayerID, Region: profile.Region}
		}
	}
	return nil
}

func refreshedPrimaryIndex(profiles []platformmihomo.DiscoveredProfile, previous *biz.ProfileIdentity) int {
	if len(profiles) == 0 {
		return -1
	}
	if previous != nil {
		for index, profile := range profiles {
			if profile.PlayerID == previous.PlayerID && profile.Region == previous.Region {
				return index
			}
		}
	}
	return 0
}

func profileSummariesFromProfiles(profiles []*biz.Profile) []*ProfileSummary {
	summaries := make([]*ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		summaries = append(summaries, toProfileSummary(profile))
	}
	return summaries
}

func (uc *StatusUsecase) getCredential(ctx context.Context, accountKey string) (*biz.Credential, error) {
	credential, err := uc.credentials.GetByAccountKey(ctx, accountKey)
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
