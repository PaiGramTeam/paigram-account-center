package usecase

import (
	"context"
	"errors"

	"platform-mihomo-service/internal/biz"
)

var ErrProfileNotFound = errors.New("profile not found")

type ProfileUsecase struct {
	profileRepo biz.ProfileRepository
}

func NewProfileUsecase(profileRepo biz.ProfileRepository) *ProfileUsecase {
	return &ProfileUsecase{profileRepo: profileRepo}
}

func (uc *ProfileUsecase) ListProfiles(ctx context.Context, accountKey string) ([]*ProfileSummary, error) {
	profiles, err := uc.profileRepo.ListByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	summaries := make([]*ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		summaries = append(summaries, toProfileSummary(profile))
	}

	return summaries, nil
}

func (uc *ProfileUsecase) ListProfilesWithScope(ctx context.Context, guard ScopeGuard, accountKey string) ([]*ProfileSummary, error) {
	profiles, err := uc.listScopedProfiles(ctx, guard, accountKey)
	if err != nil {
		return nil, err
	}

	summaries := make([]*ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		summaries = append(summaries, toProfileSummary(profile))
	}

	return summaries, nil
}

func (uc *ProfileUsecase) GetPrimaryProfile(ctx context.Context, accountKey string) (*ProfileSummary, error) {
	profiles, err := uc.profileRepo.ListByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	if len(profiles) == 0 {
		return nil, nil
	}

	for _, profile := range profiles {
		if profile.IsDefault {
			return toProfileSummary(profile), nil
		}
	}

	return toProfileSummary(profiles[0]), nil
}

func (uc *ProfileUsecase) GetPrimaryProfileWithScope(ctx context.Context, guard ScopeGuard, accountKey string) (*ProfileSummary, error) {
	profiles, err := uc.listScopedProfiles(ctx, guard, accountKey)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	for _, profile := range profiles {
		if profile.IsDefault {
			return toProfileSummary(profile), nil
		}
	}
	return toProfileSummary(profiles[0]), nil
}

func (uc *ProfileUsecase) ConfirmPrimaryProfile(ctx context.Context, accountKey string, playerID string) (*ProfileSummary, error) {
	profiles, err := uc.profileRepo.ListByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	var selected *biz.Profile
	for _, profile := range profiles {
		if profile.PlayerID == playerID {
			selected = profile
			break
		}
	}
	if selected == nil {
		return nil, ErrProfileNotFound
	}

	if err := uc.profileRepo.SetDefaultByBindingAndPlayerID(ctx, selected.BindingRef, accountKey, playerID); err != nil {
		return nil, err
	}
	selected.IsDefault = true
	return toProfileSummary(selected), nil
}

func (uc *ProfileUsecase) ConfirmPrimaryProfileWithScope(ctx context.Context, guard ScopeGuard, accountKey string, playerID string) (*ProfileSummary, error) {
	if err := guard.RequireAccountKey(accountKey); err != nil {
		return nil, err
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, err
	}
	_, selected, err := uc.findProfileByBindingAndPlayerID(ctx, guard.BindingRef, accountKey, playerID)
	if err != nil {
		return nil, err
	}
	if err := guard.RequireProfile(selected.BindingRef, selected.ProfileRef); err != nil {
		return nil, err
	}
	if err := uc.profileRepo.SetDefaultByBindingAndPlayerID(ctx, selected.BindingRef, accountKey, playerID); err != nil {
		return nil, err
	}
	selected.IsDefault = true
	return toProfileSummary(selected), nil
}

func (uc *ProfileUsecase) RequireProfileAccessByPlayerID(ctx context.Context, guard ScopeGuard, accountKey string, playerID string) error {
	if err := guard.RequireAccountKey(accountKey); err != nil {
		return err
	}
	_, selected, err := uc.findProfileByPlayerID(ctx, accountKey, playerID)
	if err != nil {
		return err
	}
	return guard.RequireProfile(selected.BindingRef, selected.ProfileRef)
}

func (uc *ProfileUsecase) GetProfileByRef(ctx context.Context, guard ScopeGuard, accountKey string, profileRef string) (*biz.Profile, error) {
	if err := guard.RequireAccountKey(accountKey); err != nil {
		return nil, err
	}
	if profileRef == "" {
		return nil, ErrProfileNotFound
	}
	if err := guard.RequireProfile(guard.BindingRef, profileRef); err != nil {
		return nil, err
	}
	profile, err := uc.profileRepo.GetByProfileRef(ctx, guard.BindingRef, profileRef)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.AccountKey != accountKey {
		return nil, ErrProfileNotFound
	}
	return profile, nil
}

func (uc *ProfileUsecase) listScopedProfiles(ctx context.Context, guard ScopeGuard, accountKey string) ([]*biz.Profile, error) {
	if err := guard.RequireAccountKey(accountKey); err != nil {
		return nil, err
	}
	profiles, err := uc.profileRepo.ListByBindingRef(ctx, guard.BindingRef)
	if err != nil {
		return nil, err
	}
	filteredByAccount := profiles[:0]
	for _, profile := range profiles {
		if profile.AccountKey == accountKey {
			filteredByAccount = append(filteredByAccount, profile)
		}
	}
	profiles = filteredByAccount
	if guard.ProfileRef == "" {
		return profiles, nil
	}
	filtered := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if guard.RequireProfile(profile.BindingRef, profile.ProfileRef) == nil {
			filtered = append(filtered, profile)
		}
	}
	if len(filtered) == 0 {
		return nil, ErrProfileScopeDenied
	}
	return filtered, nil
}

func (uc *ProfileUsecase) findProfileByPlayerID(ctx context.Context, accountKey string, playerID string) ([]*biz.Profile, *biz.Profile, error) {
	profiles, err := uc.profileRepo.ListByAccountKey(ctx, accountKey)
	if err != nil {
		return nil, nil, err
	}
	for _, profile := range profiles {
		if profile.PlayerID == playerID {
			return profiles, profile, nil
		}
	}
	return profiles, nil, ErrProfileNotFound
}

func (uc *ProfileUsecase) findProfileByBindingAndPlayerID(ctx context.Context, bindingRef string, accountKey string, playerID string) ([]*biz.Profile, *biz.Profile, error) {
	profiles, err := uc.profileRepo.ListByBindingRef(ctx, bindingRef)
	if err != nil {
		return nil, nil, err
	}
	filtered := profiles[:0]
	var selected *biz.Profile
	for _, profile := range profiles {
		if profile.AccountKey != accountKey {
			continue
		}
		filtered = append(filtered, profile)
		if profile.PlayerID == playerID {
			selected = profile
		}
	}
	if selected != nil {
		return filtered, selected, nil
	}
	return filtered, nil, ErrProfileNotFound
}
