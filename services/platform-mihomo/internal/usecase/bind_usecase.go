package usecase

import (
	"context"
	"errors"
	"time"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

type BindCredentialInput struct {
	BindingRef       string
	Generation       uint64
	CookieBundleJSON string
	DeviceID         string
	DeviceFP         string
	DeviceName       string
	RegionHint       string
}

type BindCredentialOutput struct {
	BindingRef string
	AccountKey string
	Profiles   []ProfileSummary
	Status     CredentialStatus
}

type bindCredentialPreparation struct {
	accountKey         string
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
	return uc.bindCredential(ctx, input, false)
}

func (uc *BindUsecase) BindCredentialIfAbsent(ctx context.Context, input BindCredentialInput) (*BindCredentialOutput, error) {
	return uc.bindCredential(ctx, input, true)
}

func (uc *BindUsecase) bindCredential(ctx context.Context, input BindCredentialInput, createOnly bool) (*BindCredentialOutput, error) {
	prepared, err := uc.prepareBindCredential(ctx, input)
	if err != nil {
		return nil, err
	}

	var output *BindCredentialOutput
	err = uc.runInTransaction(ctx, func(txCtx context.Context) error {
		result, err := uc.bindPreparedCredential(txCtx, input, prepared, createOnly)
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
	if input.BindingRef == "" {
		return nil, errors.New("binding_ref is required")
	}

	accountID, region, discoveredProfiles, err := uc.client.ValidateAndDiscover(ctx, input.CookieBundleJSON, input.RegionHint)
	if err != nil {
		return nil, err
	}

	accountKey := FormatAccountKey(accountID)
	encryptedBlob, err := internalcrypto.EncryptString(uc.encryptionKey, input.CookieBundleJSON)
	if err != nil {
		return nil, err
	}

	return &bindCredentialPreparation{
		accountKey:         accountKey,
		accountID:          accountID,
		region:             region,
		discoveredProfiles: discoveredProfiles,
		encryptedBlob:      encryptedBlob,
	}, nil
}

func (uc *BindUsecase) bindPreparedCredential(ctx context.Context, input BindCredentialInput, prepared *bindCredentialPreparation, createOnly bool) (*BindCredentialOutput, error) {
	if prepared == nil {
		return nil, errors.New("bind preparation is required")
	}

	existingCredential, err := uc.credentialRepo.GetByBindingRef(ctx, input.BindingRef)
	if err != nil {
		return nil, err
	}
	if createOnly && existingCredential != nil {
		return nil, biz.ErrCredentialAlreadyBound
	}
	if uc.artifactRepo != nil {
		if err := uc.artifactRepo.DeleteByBindingRef(ctx, input.BindingRef); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	profileRevision := input.Generation
	if existingCredential != nil {
		profileRevision = nextProfileRevision(existingCredential.ProfileRevision, existingCredential.ProfileObservedRevision)
	}
	credential := &biz.Credential{
		BindingRef:              input.BindingRef,
		AccountKey:              prepared.accountKey,
		Generation:              input.Generation,
		Platform:                "mihomo",
		AccountID:               prepared.accountID,
		Region:                  prepared.region,
		CredentialBlob:          prepared.encryptedBlob,
		CredentialVersion:       "v1",
		Status:                  "active",
		LastValidatedAt:         &now,
		ProfileSnapshotComplete: true,
		ProfileRevision:         profileRevision,
		ProfileObservedRevision: profileRevision,
	}
	if err := uc.persistCredential(ctx, credential, createOnly); err != nil {
		return nil, err
	}
	previousAccountKey := ""
	if existingCredential != nil && existingCredential.AccountKey != "" && existingCredential.AccountKey != prepared.accountKey {
		previousAccountKey = existingCredential.AccountKey
	}
	rollback := true
	defer func() {
		if rollback {
			_ = uc.profileRepo.DeleteByAccountKey(ctx, prepared.accountKey)
			_ = uc.deviceRepo.DeleteByAccountKey(ctx, prepared.accountKey)
			if previousAccountKey != "" {
				_ = uc.credentialRepo.Save(ctx, existingCredential)
				return
			}
			_ = uc.credentialRepo.DeleteByAccountKey(ctx, prepared.accountKey)
		}
	}()

	device := &biz.Device{
		BindingRef: input.BindingRef,
		AccountKey: prepared.accountKey,
		DeviceRef:  FormatDeviceRef(prepared.accountKey, input.DeviceID),
		DeviceID:   input.DeviceID,
		DeviceFP:   input.DeviceFP,
		IsValid:    true,
		LastSeenAt: &now,
	}
	if input.DeviceName != "" {
		deviceName := input.DeviceName
		device.DeviceName = &deviceName
	}
	if err := uc.deviceRepo.Save(ctx, device); err != nil {
		return nil, err
	}

	if previousAccountKey != "" {
		if err := uc.profileRepo.DeleteByAccountKey(ctx, previousAccountKey); err != nil {
			return nil, err
		}
	} else {
		if err := uc.profileRepo.DeleteByAccountKey(ctx, prepared.accountKey); err != nil {
			return nil, err
		}
	}
	outputProfiles := make([]ProfileSummary, 0, len(prepared.discoveredProfiles))
	for index, discoveredProfile := range prepared.discoveredProfiles {
		profile := &biz.Profile{
			BindingRef:   input.BindingRef,
			AccountKey:   prepared.accountKey,
			ProfileRef:   FormatProfileRef(prepared.accountKey, discoveredProfile.GameBiz, discoveredProfile.Region, discoveredProfile.PlayerID),
			GameBiz:      discoveredProfile.GameBiz,
			Region:       discoveredProfile.Region,
			PlayerID:     discoveredProfile.PlayerID,
			Nickname:     discoveredProfile.Nickname,
			Level:        int(discoveredProfile.Level),
			IsDefault:    index == 0,
			DiscoveredAt: now,
		}
		if err := uc.profileRepo.Save(ctx, profile); err != nil {
			return nil, err
		}

		outputProfiles = append(outputProfiles, *toProfileSummary(profile))
	}

	if previousAccountKey != "" {
		if err := uc.deviceRepo.DeleteByAccountKey(ctx, previousAccountKey); err != nil {
			return nil, err
		}
		if err := uc.credentialRepo.DeleteByAccountKey(ctx, previousAccountKey); err != nil {
			return nil, err
		}
	}

	rollback = false

	return &BindCredentialOutput{
		BindingRef: input.BindingRef,
		AccountKey: prepared.accountKey,
		Profiles:   outputProfiles,
		Status:     CredentialStatusActive,
	}, nil
}

func (uc *BindUsecase) persistCredential(ctx context.Context, credential *biz.Credential, createOnly bool) error {
	if createOnly {
		return uc.credentialRepo.Create(ctx, credential)
	}
	return uc.credentialRepo.Save(ctx, credential)
}

func (uc *BindUsecase) UpsertDevice(ctx context.Context, accountKey string, deviceID string, deviceFP string, deviceName string) error {
	credential, err := uc.credentialRepo.GetByAccountKey(ctx, accountKey)
	if err != nil {
		return err
	}
	if credential == nil {
		return ErrCredentialNotFound
	}

	device := &biz.Device{
		BindingRef: credential.BindingRef,
		AccountKey: accountKey,
		DeviceID:   deviceID,
		DeviceFP:   deviceFP,
		IsValid:    true,
	}
	if deviceName != "" {
		name := deviceName
		device.DeviceName = &name
	}
	now := time.Now().UTC()
	device.LastSeenAt = &now
	return uc.deviceRepo.Save(ctx, device)
}
