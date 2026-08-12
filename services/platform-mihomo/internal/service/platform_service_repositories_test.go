package service

import (
	"context"
	"strconv"
	"time"

	"platform-mihomo-service/internal/biz"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	mihomostub "platform-mihomo-service/internal/testkit/mihomostub"
)

type memoryManagementRepo struct {
	credentialRepo *memoryCredentialRepo
	deviceRepo     *memoryDeviceRepo
	profileRepo    *memoryProfileRepo
	artifactRepo   *memoryArtifactRepo
}

func newMemoryManagementRepo(
	credentialRepo *memoryCredentialRepo,
	deviceRepo *memoryDeviceRepo,
	profileRepo *memoryProfileRepo,
	artifactRepo *memoryArtifactRepo,
) *memoryManagementRepo {
	return &memoryManagementRepo{
		credentialRepo: credentialRepo,
		deviceRepo:     deviceRepo,
		profileRepo:    profileRepo,
		artifactRepo:   artifactRepo,
	}
}

func (r *memoryManagementRepo) DeleteCredentialGraph(_ context.Context, platformAccountID string) error {
	_ = r.credentialRepo.DeleteByPlatformAccountID(context.Background(), platformAccountID)
	_ = r.deviceRepo.DeleteByPlatformAccountID(context.Background(), platformAccountID)
	_ = r.profileRepo.DeleteByPlatformAccountID(context.Background(), platformAccountID)
	for key, artifact := range r.artifactRepo.artifacts {
		if artifact.PlatformAccountID == platformAccountID {
			delete(r.artifactRepo.artifacts, key)
		}
	}
	return nil
}

func (r *memoryManagementRepo) DeleteCredentialGraphByBindingID(_ context.Context, bindingID uint64) error {
	credential := r.credentialRepo.byBindingID[bindingID]
	if credential != nil {
		delete(r.credentialRepo.byPlatformAccountID, credential.PlatformAccountID)
		for key, artifact := range r.artifactRepo.artifacts {
			if artifact.PlatformAccountID == credential.PlatformAccountID {
				delete(r.artifactRepo.artifacts, key)
			}
		}
	}
	delete(r.credentialRepo.byBindingID, bindingID)
	_ = r.deviceRepo.DeleteByBindingID(context.Background(), bindingID)
	for _, profile := range r.profileRepo.byBindingID[bindingID] {
		delete(r.profileRepo.byPlatformAccountID, profile.PlatformAccountID)
	}
	delete(r.profileRepo.byBindingID, bindingID)
	return nil
}

type memoryCredentialRepo struct {
	byPlatformAccountID map[string]*biz.Credential
	byBindingID         map[uint64]*biz.Credential
}

func newMemoryCredentialRepo() *memoryCredentialRepo {
	return &memoryCredentialRepo{
		byPlatformAccountID: make(map[string]*biz.Credential),
		byBindingID:         make(map[uint64]*biz.Credential),
	}
}

func (r *memoryCredentialRepo) Save(_ context.Context, credential *biz.Credential) error {
	clone := *credential
	r.byPlatformAccountID[credential.PlatformAccountID] = &clone
	r.byBindingID[credential.BindingID] = &clone
	return nil
}

func (r *memoryCredentialRepo) Create(ctx context.Context, credential *biz.Credential) error {
	if r.byBindingID[credential.BindingID] != nil || r.byPlatformAccountID[credential.PlatformAccountID] != nil {
		return biz.ErrCredentialAlreadyBound
	}
	return r.Save(ctx, credential)
}

func (r *memoryCredentialRepo) GetByBindingID(_ context.Context, bindingID uint64) (*biz.Credential, error) {
	credential := r.byBindingID[bindingID]
	if credential == nil {
		return nil, nil
	}

	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) GetByPlatformAccountID(_ context.Context, platformAccountID string) (*biz.Credential, error) {
	credential := r.byPlatformAccountID[platformAccountID]
	if credential == nil {
		return nil, nil
	}

	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if credential := r.byPlatformAccountID[platformAccountID]; credential != nil {
		delete(r.byBindingID, credential.BindingID)
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

type memoryDeviceRepo struct {
	byPlatformAccountID map[string][]*biz.Device
	byBindingID         map[uint64][]*biz.Device
}

func newMemoryDeviceRepo() *memoryDeviceRepo {
	return &memoryDeviceRepo{
		byPlatformAccountID: make(map[string][]*biz.Device),
		byBindingID:         make(map[uint64][]*biz.Device),
	}
}

func (r *memoryDeviceRepo) Save(_ context.Context, device *biz.Device) error {
	clone := *device
	current := r.byPlatformAccountID[device.PlatformAccountID]
	byBinding := r.byBindingID[device.BindingID]
	for index, existing := range current {
		if existing.DeviceID == device.DeviceID {
			current[index] = &clone
			r.byPlatformAccountID[device.PlatformAccountID] = current
			for bindingIndex, bindingDevice := range byBinding {
				if bindingDevice.DeviceID == device.DeviceID {
					byBinding[bindingIndex] = &clone
					r.byBindingID[device.BindingID] = byBinding
					return nil
				}
			}
			return nil
		}
	}

	r.byPlatformAccountID[device.PlatformAccountID] = append(current, &clone)
	r.byBindingID[device.BindingID] = append(byBinding, &clone)
	return nil
}

func (r *memoryDeviceRepo) ListByBindingID(_ context.Context, bindingID uint64) ([]*biz.Device, error) {
	devices := r.byBindingID[bindingID]
	result := make([]*biz.Device, 0, len(devices))
	for _, device := range devices {
		clone := *device
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryDeviceRepo) ListByPlatformAccountID(_ context.Context, platformAccountID string) ([]*biz.Device, error) {
	devices := r.byPlatformAccountID[platformAccountID]
	result := make([]*biz.Device, 0, len(devices))
	for _, device := range devices {
		clone := *device
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryDeviceRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if devices := r.byPlatformAccountID[platformAccountID]; len(devices) > 0 {
		bindingID := devices[0].BindingID
		current := r.byBindingID[bindingID]
		filtered := make([]*biz.Device, 0, len(current))
		for _, device := range current {
			if device.PlatformAccountID != platformAccountID {
				filtered = append(filtered, device)
			}
		}
		if len(filtered) == 0 {
			delete(r.byBindingID, bindingID)
		} else {
			r.byBindingID[bindingID] = filtered
		}
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

func (r *memoryDeviceRepo) DeleteByBindingID(_ context.Context, bindingID uint64) error {
	for _, device := range r.byBindingID[bindingID] {
		delete(r.byPlatformAccountID, device.PlatformAccountID)
	}
	delete(r.byBindingID, bindingID)
	return nil
}

type memoryProfileRepo struct {
	byPlatformAccountID map[string][]*biz.Profile
	byBindingID         map[uint64][]*biz.Profile
}

func newMemoryProfileRepo() *memoryProfileRepo {
	return &memoryProfileRepo{
		byPlatformAccountID: make(map[string][]*biz.Profile),
		byBindingID:         make(map[uint64][]*biz.Profile),
	}
}

func (r *memoryProfileRepo) Save(_ context.Context, profile *biz.Profile) error {
	clone := *profile
	current := r.byPlatformAccountID[profile.PlatformAccountID]
	byBinding := r.byBindingID[profile.BindingID]
	for index, existing := range current {
		if existing.PlayerID == profile.PlayerID && existing.Region == profile.Region {
			current[index] = &clone
			r.byPlatformAccountID[profile.PlatformAccountID] = current
			for bindingIndex, bindingProfile := range byBinding {
				if bindingProfile.PlayerID == profile.PlayerID && bindingProfile.Region == profile.Region {
					byBinding[bindingIndex] = &clone
					r.byBindingID[profile.BindingID] = byBinding
					return nil
				}
			}
			return nil
		}
	}

	r.byPlatformAccountID[profile.PlatformAccountID] = append(current, &clone)
	r.byBindingID[profile.BindingID] = append(byBinding, &clone)
	return nil
}

func (r *memoryProfileRepo) ListByBindingID(_ context.Context, bindingID uint64) ([]*biz.Profile, error) {
	profiles := r.byBindingID[bindingID]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryProfileRepo) ListByPlatformAccountID(_ context.Context, platformAccountID string) ([]*biz.Profile, error) {
	profiles := r.byPlatformAccountID[platformAccountID]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}

	return result, nil
}

func (r *memoryProfileRepo) SetDefaultByBindingAndPlayerID(_ context.Context, bindingID uint64, platformAccountID string, playerID string) error {
	for _, profile := range r.byPlatformAccountID[platformAccountID] {
		if profile.BindingID == bindingID {
			profile.IsDefault = profile.PlayerID == playerID
		}
	}
	for _, profile := range r.byBindingID[bindingID] {
		if profile.PlatformAccountID == platformAccountID {
			profile.IsDefault = profile.PlayerID == playerID
		}
	}
	return nil
}

func (r *memoryProfileRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if profiles := r.byPlatformAccountID[platformAccountID]; len(profiles) > 0 {
		bindingID := profiles[0].BindingID
		current := r.byBindingID[bindingID]
		filtered := make([]*biz.Profile, 0, len(current))
		for _, profile := range current {
			if profile.PlatformAccountID != platformAccountID {
				filtered = append(filtered, profile)
			}
		}
		if len(filtered) == 0 {
			delete(r.byBindingID, bindingID)
		} else {
			r.byBindingID[bindingID] = filtered
		}
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

func (r *memoryProfileRepo) DeleteMissingByPlatformAccountID(_ context.Context, platformAccountID string, keep []biz.ProfileIdentity) error {
	profiles := r.byPlatformAccountID[platformAccountID]
	if len(profiles) == 0 {
		return nil
	}
	bindingID := profiles[0].BindingID
	keepSet := make(map[string]struct{}, len(keep))
	for _, identity := range keep {
		keepSet[identity.PlayerID+":"+identity.Region] = struct{}{}
	}
	filtered := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
			filtered = append(filtered, profile)
		}
	}
	r.byPlatformAccountID[platformAccountID] = filtered
	current := r.byBindingID[bindingID]
	filteredByBinding := make([]*biz.Profile, 0, len(current))
	for _, profile := range current {
		if profile.PlatformAccountID != platformAccountID {
			filteredByBinding = append(filteredByBinding, profile)
			continue
		}
		if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
			filteredByBinding = append(filteredByBinding, profile)
		}
	}
	if len(filteredByBinding) == 0 {
		delete(r.byBindingID, bindingID)
	} else {
		r.byBindingID[bindingID] = filteredByBinding
	}
	if len(filtered) == 0 {
		delete(r.byPlatformAccountID, platformAccountID)
	}
	return nil
}

func (r *memoryProfileRepo) DeleteMissingByBindingID(_ context.Context, bindingID uint64, keep []biz.ProfileIdentity) error {
	profiles := r.byBindingID[bindingID]
	keepSet := make(map[string]struct{}, len(keep))
	for _, identity := range keep {
		keepSet[identity.PlayerID+":"+identity.Region] = struct{}{}
	}
	filtered := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
			filtered = append(filtered, profile)
		}
	}
	r.byBindingID[bindingID] = filtered
	for platformAccountID, platformProfiles := range r.byPlatformAccountID {
		filteredByAccount := platformProfiles[:0]
		for _, profile := range platformProfiles {
			if profile.BindingID != bindingID {
				filteredByAccount = append(filteredByAccount, profile)
				continue
			}
			if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
				filteredByAccount = append(filteredByAccount, profile)
			}
		}
		if len(filteredByAccount) == 0 {
			delete(r.byPlatformAccountID, platformAccountID)
		} else {
			r.byPlatformAccountID[platformAccountID] = filteredByAccount
		}
	}
	return nil
}

type memoryArtifactRepo struct {
	artifacts map[string]*biz.Artifact
}

func newMemoryArtifactRepo() *memoryArtifactRepo {
	return &memoryArtifactRepo{artifacts: make(map[string]*biz.Artifact)}
}

func (r *memoryArtifactRepo) Put(_ context.Context, artifact *biz.Artifact) error {
	clone := *artifact
	r.artifacts[bindingArtifactKey(artifact.BindingID, artifact.ArtifactType, artifact.ScopeKey)] = &clone
	return nil
}

func (r *memoryArtifactRepo) GetByBindingID(_ context.Context, bindingID uint64, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact := r.artifacts[bindingArtifactKey(bindingID, artifactType, scopeKey)]
	if artifact == nil || !artifact.ExpiresAt.After(time.Now()) {
		return nil, nil
	}

	clone := *artifact
	return &clone, nil
}

func (r *memoryArtifactRepo) Get(_ context.Context, platformAccountID, artifactType, scopeKey string) (*biz.Artifact, error) {
	artifact := r.artifacts[artifactKey(platformAccountID, artifactType, scopeKey)]
	if artifact == nil || !artifact.ExpiresAt.After(time.Now()) {
		return nil, nil
	}

	clone := *artifact
	return &clone, nil
}

func (r *memoryArtifactRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	for key, artifact := range r.artifacts {
		if artifact.PlatformAccountID == platformAccountID {
			delete(r.artifacts, key)
		}
	}
	return nil
}

func (r *memoryArtifactRepo) DeleteByBindingID(_ context.Context, bindingID uint64) error {
	for key, artifact := range r.artifacts {
		if artifact.BindingID == bindingID {
			delete(r.artifacts, key)
		}
	}
	return nil
}

func artifactKey(platformAccountID, artifactType, scopeKey string) string {
	return platformAccountID + ":" + artifactType + ":" + scopeKey
}

func bindingArtifactKey(bindingID uint64, artifactType, scopeKey string) string {
	return strconv.FormatUint(bindingID, 10) + ":" + artifactType + ":" + scopeKey
}

var _ biz.CredentialRepository = (*memoryCredentialRepo)(nil)
var _ biz.DeviceRepository = (*memoryDeviceRepo)(nil)
var _ biz.ProfileRepository = (*memoryProfileRepo)(nil)
var _ biz.ArtifactRepository = (*memoryArtifactRepo)(nil)
var _ biz.CredentialManagementRepository = (*memoryManagementRepo)(nil)
var _ platformmihomo.Client = mihomostub.Client{}
