package usecase

import (
	"context"
	"errors"
	"time"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
)

var ErrPlatformAccountMismatch = errors.New("platform account does not match requested credential")

type CredentialSummaryOutput struct {
	PlatformAccountID string
	Status            v1.CredentialStatus
	LastValidatedAt   *time.Time
	LastRefreshedAt   *time.Time
	Devices           []*biz.Device
	Profiles          []*v1.ProfileSummary
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

func (uc *ManagementUsecase) GetCredentialSummary(ctx context.Context, platformAccountID string) (*CredentialSummaryOutput, error) {
	credential, err := uc.credentials.GetByPlatformAccountID(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrCredentialNotFound
	}

	devices, err := uc.devices.ListByPlatformAccountID(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}

	profiles, err := uc.profileUC.ListProfiles(ctx, platformAccountID)
	if err != nil {
		return nil, err
	}

	return &CredentialSummaryOutput{
		PlatformAccountID: credential.PlatformAccountID,
		Status:            toCredentialStatus(credential.Status),
		LastValidatedAt:   credential.LastValidatedAt,
		LastRefreshedAt:   credential.LastRefreshedAt,
		Devices:           devices,
		Profiles:          profiles,
	}, nil
}

func (uc *ManagementUsecase) GetCredentialSummaryWithScope(ctx context.Context, guard ScopeGuard, platformAccountID string) (*CredentialSummaryOutput, error) {
	if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
		return nil, err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, err
	}
	credential, err := uc.credentials.GetByBindingID(ctx, guard.BindingID)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, ErrCredentialNotFound
	}
	if credential.PlatformAccountID != platformAccountID {
		return nil, ErrPlatformAccountMismatch
	}
	devices, err := uc.devices.ListByBindingID(ctx, guard.BindingID)
	if err != nil {
		return nil, err
	}
	profiles, err := uc.profiles.ListByBindingID(ctx, guard.BindingID)
	if err != nil {
		return nil, err
	}
	summaries := make([]*v1.ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		summaries = append(summaries, toProfileSummary(profile))
	}
	return &CredentialSummaryOutput{
		PlatformAccountID: credential.PlatformAccountID,
		Status:            toCredentialStatus(credential.Status),
		LastValidatedAt:   credential.LastValidatedAt,
		LastRefreshedAt:   credential.LastRefreshedAt,
		Devices:           devices,
		Profiles:          summaries,
	}, nil
}

type UpdateCredentialInput struct {
	PlatformAccountID string
	BindCredentialInput
}

func (uc *ManagementUsecase) UpdateCredential(ctx context.Context, input UpdateCredentialInput) (*CredentialSummaryOutput, error) {
	credential, err := uc.credentials.GetByPlatformAccountID(ctx, input.PlatformAccountID)
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
	if prepared.platformAccountID != input.PlatformAccountID {
		return nil, ErrPlatformAccountMismatch
	}

	err = uc.bindUC.runInTransaction(ctx, func(txCtx context.Context) error {
		result, err := uc.bindUC.bindPreparedCredential(txCtx, input.BindCredentialInput, prepared)
		if err != nil {
			return err
		}
		if err := uc.pruneStaleProfiles(txCtx, input.PlatformAccountID, result.Profiles); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return uc.GetCredentialSummary(ctx, input.PlatformAccountID)
}

func (uc *ManagementUsecase) UpdateCredentialWithScope(ctx context.Context, guard ScopeGuard, input UpdateCredentialInput) (*CredentialSummaryOutput, error) {
	if err := guard.RequirePlatformAccountID(input.PlatformAccountID); err != nil {
		return nil, err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, err
	}
	return uc.UpdateCredential(ctx, input)
}

func (uc *ManagementUsecase) DeleteCredential(ctx context.Context, platformAccountID string) error {
	return uc.management.DeleteCredentialGraph(ctx, platformAccountID)
}

func (uc *ManagementUsecase) DeleteCredentialWithScope(ctx context.Context, guard ScopeGuard, platformAccountID string) error {
	if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
		return err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return err
	}
	credential, err := uc.credentials.GetByBindingID(ctx, guard.BindingID)
	if err != nil {
		return err
	}
	if credential == nil {
		return ErrCredentialNotFound
	}
	if credential.PlatformAccountID != platformAccountID {
		return ErrPlatformAccountMismatch
	}
	return uc.management.DeleteCredentialGraphByBindingID(ctx, guard.BindingID)
}

func (uc *ManagementUsecase) pruneStaleProfiles(ctx context.Context, platformAccountID string, profiles []v1.ProfileSummary) error {
	keep := make([]biz.ProfileIdentity, 0, len(profiles))
	for i := range profiles {
		profile := &profiles[i]
		keep = append(keep, biz.ProfileIdentity{PlayerID: profile.PlayerId, Region: profile.Region})
	}
	bindingID, err := BindingIDFromPlatformAccountID(platformAccountID)
	if err != nil {
		return err
	}
	return uc.profiles.DeleteMissingByBindingID(ctx, bindingID, keep)
}
