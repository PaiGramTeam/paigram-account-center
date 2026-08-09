package usecase

import (
	"context"
	"errors"
	"time"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

type BindCredentialInput struct {
	BindingID        uint64
	CookieBundleJSON string
	DeviceID         string
	DeviceFP         string
	DeviceName       string
	RegionHint       string
}

type BindCredentialOutput struct {
	BindingID         uint64
	PlatformAccountID string
	Profiles          []v1.ProfileSummary
	Status            v1.CredentialStatus
}

type bindCredentialPreparation struct {
	platformAccountID  string
	accountID          string
	region             string
	discoveredProfiles []platformmihomo.DiscoveredProfile
	encryptedBlob      string
}

type BindUsecase struct {
	credentialRepo biz.CredentialRepository
	deviceRepo     biz.DeviceRepository
	profileRepo    biz.ProfileRepository
	artifactRepo   biz.ArtifactRepository
	client         platformmihomo.Client
	encryptionKey  []byte
}

type bindTransactioner interface {
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

func NewBindUsecase(
	credentialRepo biz.CredentialRepository,
	deviceRepo biz.DeviceRepository,
	profileRepo biz.ProfileRepository,
	client platformmihomo.Client,
	encryptionKey []byte,
	artifactRepo biz.ArtifactRepository,
) *BindUsecase {
	return &BindUsecase{
		credentialRepo: credentialRepo,
		deviceRepo:     deviceRepo,
		profileRepo:    profileRepo,
		artifactRepo:   artifactRepo,
		client:         client,
		encryptionKey:  encryptionKey,
	}
}

func (uc *BindUsecase) BindCredential(ctx context.Context, input BindCredentialInput) (*BindCredentialOutput, error) {
	prepared, err := uc.prepareBindCredential(ctx, input)
	if err != nil {
		return nil, err
	}

	var output *BindCredentialOutput
	err = uc.runInTransaction(ctx, func(txCtx context.Context) error {
		result, err := uc.bindPreparedCredential(txCtx, input, prepared)
		if err != nil {
			return err
		}
		output = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (uc *BindUsecase) runInTransaction(ctx context.Context, fn func(context.Context) error) error {
	if txRepo, ok := uc.credentialRepo.(bindTransactioner); ok {
		return txRepo.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}

func (uc *BindUsecase) prepareBindCredential(ctx context.Context, input BindCredentialInput) (*bindCredentialPreparation, error) {
	if input.BindingID == 0 {
		return nil, errors.New("binding id is required")
	}

	accountID, region, discoveredProfiles, err := uc.client.ValidateAndDiscover(ctx, input.CookieBundleJSON, input.RegionHint)
	if err != nil {
		return nil, err
	}

	platformAccountID := FormatPlatformAccountID(input.BindingID, accountID)
	encryptedBlob, err := internalcrypto.EncryptString(uc.encryptionKey, input.CookieBundleJSON)
	if err != nil {
		return nil, err
	}

	return &bindCredentialPreparation{
		platformAccountID:  platformAccountID,
		accountID:          accountID,
		region:             region,
		discoveredProfiles: discoveredProfiles,
		encryptedBlob:      encryptedBlob,
	}, nil
}

func (uc *BindUsecase) bindPreparedCredential(ctx context.Context, input BindCredentialInput, prepared *bindCredentialPreparation) (*BindCredentialOutput, error) {
	if prepared == nil {
		return nil, errors.New("bind preparation is required")
	}

	existingCredential, err := uc.credentialRepo.GetByBindingID(ctx, input.BindingID)
	if err != nil {
		return nil, err
	}
	if uc.artifactRepo != nil {
		if err := uc.artifactRepo.DeleteByBindingID(ctx, input.BindingID); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	if err := uc.credentialRepo.Save(ctx, &biz.Credential{
		BindingID:         input.BindingID,
		PlatformAccountID: prepared.platformAccountID,
		Platform:          "mihomo",
		AccountID:         prepared.accountID,
		Region:            prepared.region,
		CredentialBlob:    prepared.encryptedBlob,
		CredentialVersion: "v1",
		Status:            "active",
		LastValidatedAt:   &now,
	}); err != nil {
		return nil, err
	}
	previousPlatformAccountID := ""
	if existingCredential != nil && existingCredential.PlatformAccountID != "" && existingCredential.PlatformAccountID != prepared.platformAccountID {
		previousPlatformAccountID = existingCredential.PlatformAccountID
	}
	rollback := true
	defer func() {
		if rollback {
			_ = uc.profileRepo.DeleteByPlatformAccountID(ctx, prepared.platformAccountID)
			_ = uc.deviceRepo.DeleteByPlatformAccountID(ctx, prepared.platformAccountID)
			if previousPlatformAccountID != "" {
				_ = uc.credentialRepo.Save(ctx, existingCredential)
				return
			}
			_ = uc.credentialRepo.DeleteByPlatformAccountID(ctx, prepared.platformAccountID)
		}
	}()

	device := &biz.Device{
		BindingID:         input.BindingID,
		PlatformAccountID: prepared.platformAccountID,
		DeviceID:          input.DeviceID,
		DeviceFP:          input.DeviceFP,
		IsValid:           true,
		LastSeenAt:        &now,
	}
	if input.DeviceName != "" {
		deviceName := input.DeviceName
		device.DeviceName = &deviceName
	}
	if err := uc.deviceRepo.Save(ctx, device); err != nil {
		return nil, err
	}

	if previousPlatformAccountID != "" {
		if err := uc.profileRepo.DeleteByPlatformAccountID(ctx, previousPlatformAccountID); err != nil {
			return nil, err
		}
	} else {
		if err := uc.profileRepo.DeleteByPlatformAccountID(ctx, prepared.platformAccountID); err != nil {
			return nil, err
		}
	}
	outputProfiles := make([]v1.ProfileSummary, 0, len(prepared.discoveredProfiles))
	for index, discoveredProfile := range prepared.discoveredProfiles {
		profile := &biz.Profile{
			BindingID:         input.BindingID,
			PlatformAccountID: prepared.platformAccountID,
			GameBiz:           discoveredProfile.GameBiz,
			Region:            discoveredProfile.Region,
			PlayerID:          discoveredProfile.PlayerID,
			Nickname:          discoveredProfile.Nickname,
			Level:             int(discoveredProfile.Level),
			IsDefault:         index == 0,
			DiscoveredAt:      now,
		}
		if err := uc.profileRepo.Save(ctx, profile); err != nil {
			return nil, err
		}

		outputProfiles = append(outputProfiles, *toProfileSummary(profile))
	}

	if previousPlatformAccountID != "" {
		if err := uc.deviceRepo.DeleteByPlatformAccountID(ctx, previousPlatformAccountID); err != nil {
			return nil, err
		}
		if err := uc.credentialRepo.DeleteByPlatformAccountID(ctx, previousPlatformAccountID); err != nil {
			return nil, err
		}
	}

	rollback = false

	return &BindCredentialOutput{
		BindingID:         input.BindingID,
		PlatformAccountID: prepared.platformAccountID,
		Profiles:          outputProfiles,
		Status:            v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
	}, nil
}

func (uc *BindUsecase) UpsertDevice(ctx context.Context, platformAccountID string, deviceID string, deviceFP string, deviceName string) error {
	credential, err := uc.credentialRepo.GetByPlatformAccountID(ctx, platformAccountID)
	if err != nil {
		return err
	}
	if credential == nil {
		return ErrCredentialNotFound
	}

	device := &biz.Device{
		BindingID:         credential.BindingID,
		PlatformAccountID: platformAccountID,
		DeviceID:          deviceID,
		DeviceFP:          deviceFP,
		IsValid:           true,
	}
	if deviceName != "" {
		name := deviceName
		device.DeviceName = &name
	}
	now := time.Now().UTC()
	device.LastSeenAt = &now
	return uc.deviceRepo.Save(ctx, device)
}
