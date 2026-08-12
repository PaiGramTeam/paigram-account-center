package usecase

import (
	"context"
	"errors"
	"time"

	"platform-mihomo-service/internal/biz"
)

var ErrPlatformAccountMismatch = errors.New("platform account does not match requested credential")

type CredentialSummaryOutput struct {
	AccountKey              string
	Generation              uint64
	Status                  CredentialStatus
	LastValidatedAt         *time.Time
	LastRefreshedAt         *time.Time
	Devices                 []*biz.Device
	Profiles                []*ProfileSummary
	ProfileSnapshotComplete bool
	ProfileRevision         uint64
	ProfileObservedRevision uint64
}

type ManagementUsecase struct {
	credentials biz.CredentialRepository
	devices     biz.DeviceRepository
	profiles    biz.ProfileRepository
	artifacts   biz.ArtifactRepository
	management  biz.CredentialManagementRepository
	bindUC      *BindUsecase
	profileUC   *ProfileUsecase
}

func NewManagementUsecase(
	credentials biz.CredentialRepository,
	devices biz.DeviceRepository,
	profiles biz.ProfileRepository,
	artifacts biz.ArtifactRepository,
	management biz.CredentialManagementRepository,
	bindUC *BindUsecase,
	profileUC *ProfileUsecase,
) *ManagementUsecase {
	return &ManagementUsecase{
		credentials: credentials,
		devices:     devices,
		profiles:    profiles,
		artifacts:   artifacts,
		management:  management,
		bindUC:      bindUC,
		profileUC:   profileUC,
	}
}

func (uc *ManagementUsecase) GetCredentialSummary(ctx context.Context, accountKey string) (*CredentialSummaryOutput, error) {
	credential, err := uc.credentials.GetByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrCredentialNotFound
	}

	devices, err := uc.devices.ListByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	profiles, err := uc.profileUC.ListProfiles(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	return &CredentialSummaryOutput{
		AccountKey:              credential.AccountKey,
		Generation:              credential.Generation,
		Status:                  credentialStatusFromStorage(credential.Status),
		LastValidatedAt:         credential.LastValidatedAt,
		LastRefreshedAt:         credential.LastRefreshedAt,
		Devices:                 devices,
		Profiles:                profiles,
		ProfileSnapshotComplete: credential.ProfileSnapshotComplete,
		ProfileRevision:         credential.ProfileRevision,
		ProfileObservedRevision: credential.ProfileObservedRevision,
	}, nil
}

func (uc *ManagementUsecase) GetCredentialSummaryWithScope(ctx context.Context, guard ScopeGuard, accountKey string) (*CredentialSummaryOutput, error) {
	if err := guard.RequireAccountKey(accountKey); err != nil {
		return nil, err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, err
	}
	credential, err := uc.credentials.GetByBindingRef(ctx, guard.BindingRef)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrBindingScopeDenied
	}
	if credential.AccountKey != accountKey {
		return nil, ErrPlatformAccountMismatch
	}
	devices, err := uc.devices.ListByBindingRef(ctx, guard.BindingRef)
	if err != nil {
		return nil, err
	}
	profiles, err := uc.profiles.ListByBindingRef(ctx, guard.BindingRef)
	if err != nil {
		return nil, err
	}
	summaries := make([]*ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		summaries = append(summaries, toProfileSummary(profile))
	}
	return &CredentialSummaryOutput{
		AccountKey:              credential.AccountKey,
		Generation:              credential.Generation,
		Status:                  credentialStatusFromStorage(credential.Status),
		LastValidatedAt:         credential.LastValidatedAt,
		LastRefreshedAt:         credential.LastRefreshedAt,
		Devices:                 devices,
		Profiles:                summaries,
		ProfileSnapshotComplete: credential.ProfileSnapshotComplete,
		ProfileRevision:         credential.ProfileRevision,
		ProfileObservedRevision: credential.ProfileObservedRevision,
	}, nil
}

type UpdateCredentialInput struct {
	AccountKey string
	BindCredentialInput
}

func (uc *ManagementUsecase) UpdateCredential(ctx context.Context, input UpdateCredentialInput) (*CredentialSummaryOutput, error) {
	credential, err := uc.credentials.GetByAccountKey(ctx, input.AccountKey)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrCredentialNotFound
	}

	prepared, err := uc.bindUC.prepareBindCredential(ctx, input.BindCredentialInput)
	if err != nil {
		return nil, err
	}
	if prepared.accountKey != input.AccountKey {
		return nil, ErrPlatformAccountMismatch
	}

	err = uc.bindUC.runInTransaction(ctx, func(txCtx context.Context) error {
		result, err := uc.bindUC.bindPreparedCredential(txCtx, input.BindCredentialInput, prepared, false)
		if err != nil {
			return err
		}
		if err := uc.pruneStaleProfiles(txCtx, input.AccountKey, result.Profiles); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return uc.GetCredentialSummary(ctx, input.AccountKey)
}

func (uc *ManagementUsecase) UpdateCredentialWithScope(ctx context.Context, guard ScopeGuard, input UpdateCredentialInput) (*CredentialSummaryOutput, error) {
	if err := guard.RequireAccountKey(input.AccountKey); err != nil {
		return nil, err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, err
	}
	return uc.UpdateCredential(ctx, input)
}

func (uc *ManagementUsecase) DeleteCredential(ctx context.Context, accountKey string) error {
	return uc.management.DeleteCredentialGraph(ctx, accountKey)
}

func (uc *ManagementUsecase) DeleteCredentialWithScope(ctx context.Context, guard ScopeGuard, accountKey string) error {
	if err := guard.RequireAccountKey(accountKey); err != nil {
		return err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return err
	}
	credential, err := uc.credentials.GetByBindingRef(ctx, guard.BindingRef)
	if err != nil {
		return err
	}
	if credential == nil {
		return ErrBindingScopeDenied
	}
	if credential.AccountKey != accountKey {
		return ErrPlatformAccountMismatch
	}
	return uc.management.DeleteCredentialGraphByBindingRef(ctx, guard.BindingRef)
}

func (uc *ManagementUsecase) pruneStaleProfiles(ctx context.Context, accountKey string, profiles []ProfileSummary) error {
	keep := make([]biz.ProfileIdentity, 0, len(profiles))
	for i := range profiles {
		profile := &profiles[i]
		keep = append(keep, biz.ProfileIdentity{PlayerID: profile.PlayerID, Region: profile.Region})
	}
	credential, err := uc.credentials.GetByAccountKey(ctx, accountKey)
	if err != nil {
		return err
	}
	if credential == nil {
		return ErrCredentialNotFound
	}
	return uc.profiles.DeleteMissingByBindingRef(ctx, credential.BindingRef, keep)
}
