package usecase

import (
	"context"
	"errors"

	"platform-mihomo-service/internal/biz"
)

type memoryCredentialRepo struct {
	byAccountKey  map[string]*biz.Credential
	byBindingRef  map[string]*biz.Credential
	deviceRepo    *memoryDeviceRepo
	profileRepo   *memoryProfileRepo
	artifactRepo  *memoryArtifactRepo
	inTransaction bool
}

func newMemoryCredentialRepo() *memoryCredentialRepo {
	return &memoryCredentialRepo{
		byAccountKey: make(map[string]*biz.Credential),
		byBindingRef: make(map[string]*biz.Credential),
	}
}

func (r *memoryCredentialRepo) Save(_ context.Context, credential *biz.Credential) error {
	clone := *credential
	r.byAccountKey[credential.AccountKey] = &clone
	r.byBindingRef[credential.BindingRef] = &clone
	return nil
}

func (r *memoryCredentialRepo) Create(ctx context.Context, credential *biz.Credential) error {
	if r.byBindingRef[credential.BindingRef] != nil || r.byAccountKey[credential.AccountKey] != nil {
		return biz.ErrCredentialAlreadyBound
	}
	return r.Save(ctx, credential)
}

func (r *memoryCredentialRepo) AdvanceGeneration(_ context.Context, bindingRef, accountKey string, expected, target uint64) (*biz.Credential, error) {
	credential := r.byAccountKey[accountKey]
	if credential == nil || credential.BindingRef != bindingRef || credential.Generation != expected {
		return nil, biz.ErrCredentialGenerationConflict
	}
	copy := *credential
	copy.Generation = target
	r.byAccountKey[accountKey] = &copy
	r.byBindingRef[bindingRef] = &copy
	result := copy
	return &result, nil
}

func (r *memoryCredentialRepo) SetProfileSnapshotState(_ context.Context, bindingRef string, complete bool, revision, observedRevision uint64) error {
	credential := r.byBindingRef[bindingRef]
	if credential == nil {
		return biz.ErrCredentialGenerationConflict
	}
	copy := *credential
	copy.ProfileSnapshotComplete = complete
	copy.ProfileRevision = revision
	copy.ProfileObservedRevision = observedRevision
	r.byBindingRef[bindingRef] = &copy
	r.byAccountKey[copy.AccountKey] = &copy
	return nil
}

func (r *memoryCredentialRepo) GetByAccountKey(_ context.Context, accountKey string) (*biz.Credential, error) {
	credential := r.byAccountKey[accountKey]
	if credential == nil {
		return nil, nil
	}
	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) GetByBindingRef(_ context.Context, bindingRef string) (*biz.Credential, error) {
	credential := r.byBindingRef[bindingRef]
	if credential == nil {
		return nil, nil
	}
	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) DeleteByAccountKey(_ context.Context, accountKey string) error {
	if credential := r.byAccountKey[accountKey]; credential != nil {
		if current := r.byBindingRef[credential.BindingRef]; current != nil && current.AccountKey == accountKey {
			delete(r.byBindingRef, credential.BindingRef)
		}
	}
	delete(r.byAccountKey, accountKey)
	return nil
}

func (r *memoryCredentialRepo) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	credentialByPlatform := cloneCredentialMapByAccountKey(r.byAccountKey)
	credentialByBinding := cloneCredentialMapByBindingRef(r.byBindingRef)
	deviceByPlatform := cloneDeviceMapByAccountKey(r.deviceRepo.byAccountKey)
	deviceByBinding := cloneDeviceMapByBindingRef(r.deviceRepo.byBindingRef)
	profileByPlatform := cloneProfileMapByAccountKey(r.profileRepo.byAccountKey)
	profileByBinding := cloneProfileMapByBindingRef(r.profileRepo.byBindingRef)
	artifactByKey := cloneArtifactMap(r.artifactRepo.artifacts)
	r.inTransaction = true
	defer func() {
		r.inTransaction = false
	}()
	if err := fn(ctx); err != nil {
		r.byAccountKey = credentialByPlatform
		r.byBindingRef = credentialByBinding
		r.deviceRepo.byAccountKey = deviceByPlatform
		r.deviceRepo.byBindingRef = deviceByBinding
		r.profileRepo.byAccountKey = profileByPlatform
		r.profileRepo.byBindingRef = profileByBinding
		r.artifactRepo.artifacts = artifactByKey
		return err
	}
	return nil
}

type memoryDeviceRepo struct {
	byAccountKey           map[string][]*biz.Device
	byBindingRef           map[string][]*biz.Device
	failDeleteByAccountKey map[string]error
}

func newMemoryDeviceRepo() *memoryDeviceRepo {
	return &memoryDeviceRepo{
		byAccountKey:           make(map[string][]*biz.Device),
		byBindingRef:           make(map[string][]*biz.Device),
		failDeleteByAccountKey: make(map[string]error),
	}
}

func (r *memoryDeviceRepo) Save(_ context.Context, device *biz.Device) error {
	clone := *device
	current := r.byAccountKey[device.AccountKey]
	byBinding := r.byBindingRef[device.BindingRef]
	for index, existing := range current {
		if existing.DeviceID == device.DeviceID {
			current[index] = &clone
			r.byAccountKey[device.AccountKey] = current
			for bindingIndex, bindingDevice := range byBinding {
				if bindingDevice.DeviceID == device.DeviceID {
					byBinding[bindingIndex] = &clone
					r.byBindingRef[device.BindingRef] = byBinding
					return nil
				}
			}
			return nil
		}
	}
	r.byAccountKey[device.AccountKey] = append(current, &clone)
	r.byBindingRef[device.BindingRef] = append(byBinding, &clone)
	return nil
}

func (r *memoryDeviceRepo) ListByBindingRef(_ context.Context, bindingRef string) ([]*biz.Device, error) {
	devices := r.byBindingRef[bindingRef]
	result := make([]*biz.Device, 0, len(devices))
	for _, device := range devices {
		clone := *device
		result = append(result, &clone)
	}
	return result, nil
}

func (r *memoryDeviceRepo) GetByDeviceRef(_ context.Context, bindingRef string, deviceRef string) (*biz.Device, error) {
	for _, device := range r.byBindingRef[bindingRef] {
		if device.DeviceRef == deviceRef {
			clone := *device
			return &clone, nil
		}
	}
	return nil, nil
}

func (r *memoryDeviceRepo) ListByAccountKey(_ context.Context, accountKey string) ([]*biz.Device, error) {
	devices := r.byAccountKey[accountKey]
	result := make([]*biz.Device, 0, len(devices))
	for _, device := range devices {
		clone := *device
		result = append(result, &clone)
	}
	return result, nil
}

func (r *memoryDeviceRepo) DeleteByAccountKey(_ context.Context, accountKey string) error {
	if err := r.failDeleteByAccountKey[accountKey]; err != nil {
		return err
	}
	if devices := r.byAccountKey[accountKey]; len(devices) > 0 {
		bindingRef := devices[0].BindingRef
		current := r.byBindingRef[bindingRef]
		filtered := make([]*biz.Device, 0, len(current))
		for _, device := range current {
			if device.AccountKey != accountKey {
				filtered = append(filtered, device)
			}
		}
		if len(filtered) == 0 {
			delete(r.byBindingRef, bindingRef)
		} else {
			r.byBindingRef[bindingRef] = filtered
		}
	}
	delete(r.byAccountKey, accountKey)
	return nil
}

func (r *memoryDeviceRepo) DeleteByBindingRef(_ context.Context, bindingRef string) error {
	for _, device := range r.byBindingRef[bindingRef] {
		delete(r.byAccountKey, device.AccountKey)
	}
	delete(r.byBindingRef, bindingRef)
	return nil
}

type memoryProfileRepo struct {
	byAccountKey map[string][]*biz.Profile
	byBindingRef map[string][]*biz.Profile
	failSave     bool
}

func newMemoryProfileRepo() *memoryProfileRepo {
	return &memoryProfileRepo{
		byAccountKey: make(map[string][]*biz.Profile),
		byBindingRef: make(map[string][]*biz.Profile),
	}
}

func (r *memoryProfileRepo) Save(_ context.Context, profile *biz.Profile) error {
	if r.failSave {
		return errors.New("save profile failed")
	}
	if profile.IsDefault && r.hasConflictingDefault(profile) {
		return errors.New("default profile already exists for binding")
	}
	clone := *profile
	current := r.byAccountKey[profile.AccountKey]
	byBinding := r.byBindingRef[profile.BindingRef]
	for index, existing := range current {
		if existing.PlayerID == profile.PlayerID && existing.Region == profile.Region {
			current[index] = &clone
			r.byAccountKey[profile.AccountKey] = current
			for bindingIndex, bindingProfile := range byBinding {
				if bindingProfile.PlayerID == profile.PlayerID && bindingProfile.Region == profile.Region {
					byBinding[bindingIndex] = &clone
					r.byBindingRef[profile.BindingRef] = byBinding
					return nil
				}
			}
			return nil
		}
	}
	r.byAccountKey[profile.AccountKey] = append(current, &clone)
	r.byBindingRef[profile.BindingRef] = append(byBinding, &clone)
	return nil
}

func (r *memoryProfileRepo) hasConflictingDefault(profile *biz.Profile) bool {
	for _, existing := range r.byBindingRef[profile.BindingRef] {
		if !existing.IsDefault {
			continue
		}
		if existing.PlayerID == profile.PlayerID && existing.Region == profile.Region {
			continue
		}
		return true
	}
	return false
}

func (r *memoryProfileRepo) ListByAccountKey(_ context.Context, accountKey string) ([]*biz.Profile, error) {
	profiles := r.byAccountKey[accountKey]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}
	return result, nil
}

func (r *memoryProfileRepo) SetDefaultByBindingAndPlayerID(_ context.Context, bindingRef string, accountKey string, playerID string) error {
	for _, profile := range r.byAccountKey[accountKey] {
		if profile.BindingRef == bindingRef {
			profile.IsDefault = profile.PlayerID == playerID
		}
	}
	for _, profile := range r.byBindingRef[bindingRef] {
		if profile.AccountKey == accountKey {
			profile.IsDefault = profile.PlayerID == playerID
		}
	}
	return nil
}

func (r *memoryProfileRepo) ListByBindingRef(_ context.Context, bindingRef string) ([]*biz.Profile, error) {
	profiles := r.byBindingRef[bindingRef]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}
	return result, nil
}

func (r *memoryProfileRepo) GetByProfileRef(_ context.Context, bindingRef string, profileRef string) (*biz.Profile, error) {
	for _, profile := range r.byBindingRef[bindingRef] {
		if profile.ProfileRef == profileRef {
			clone := *profile
			return &clone, nil
		}
	}
	return nil, nil
}

func (r *memoryProfileRepo) DeleteByAccountKey(_ context.Context, accountKey string) error {
	if profiles := r.byAccountKey[accountKey]; len(profiles) > 0 {
		bindingRef := profiles[0].BindingRef
		current := r.byBindingRef[bindingRef]
		filtered := make([]*biz.Profile, 0, len(current))
		for _, profile := range current {
			if profile.AccountKey != accountKey {
				filtered = append(filtered, profile)
			}
		}
		if len(filtered) == 0 {
			delete(r.byBindingRef, bindingRef)
		} else {
			r.byBindingRef[bindingRef] = filtered
		}
	}
	delete(r.byAccountKey, accountKey)
	return nil
}

func (r *memoryProfileRepo) DeleteMissingByAccountKey(_ context.Context, accountKey string, keep []biz.ProfileIdentity) error {
	profiles := r.byAccountKey[accountKey]
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
	r.byAccountKey[accountKey] = filtered
	if len(filtered) == 0 {
		if len(profiles) > 0 {
			delete(r.byBindingRef, profiles[0].BindingRef)
		}
		return nil
	}
	r.byBindingRef[filtered[0].BindingRef] = filtered
	return nil
}

func (r *memoryProfileRepo) DeleteMissingByBindingRef(_ context.Context, bindingRef string, keep []biz.ProfileIdentity) error {
	profiles := r.byBindingRef[bindingRef]
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
	r.byBindingRef[bindingRef] = filtered
	for accountKey, platformProfiles := range r.byAccountKey {
		filteredByAccount := platformProfiles[:0]
		for _, profile := range platformProfiles {
			if profile.BindingRef != bindingRef {
				filteredByAccount = append(filteredByAccount, profile)
				continue
			}
			if _, ok := keepSet[profile.PlayerID+":"+profile.Region]; ok {
				filteredByAccount = append(filteredByAccount, profile)
			}
		}
		if len(filteredByAccount) == 0 {
			delete(r.byAccountKey, accountKey)
		} else {
			r.byAccountKey[accountKey] = filteredByAccount
		}
	}
	return nil
}

var _ biz.CredentialRepository = (*memoryCredentialRepo)(nil)
var _ biz.DeviceRepository = (*memoryDeviceRepo)(nil)
var _ biz.ProfileRepository = (*memoryProfileRepo)(nil)

func cloneCredentialMapByAccountKey(source map[string]*biz.Credential) map[string]*biz.Credential {
	cloned := make(map[string]*biz.Credential, len(source))
	for key, credential := range source {
		clone := *credential
		cloned[key] = &clone
	}
	return cloned
}

func cloneCredentialMapByBindingRef(source map[string]*biz.Credential) map[string]*biz.Credential {
	cloned := make(map[string]*biz.Credential, len(source))
	for key, credential := range source {
		clone := *credential
		cloned[key] = &clone
	}
	return cloned
}

func cloneDeviceMapByAccountKey(source map[string][]*biz.Device) map[string][]*biz.Device {
	cloned := make(map[string][]*biz.Device, len(source))
	for key, devices := range source {
		copied := make([]*biz.Device, 0, len(devices))
		for _, device := range devices {
			clone := *device
			copied = append(copied, &clone)
		}
		cloned[key] = copied
	}
	return cloned
}

func cloneDeviceMapByBindingRef(source map[string][]*biz.Device) map[string][]*biz.Device {
	cloned := make(map[string][]*biz.Device, len(source))
	for key, devices := range source {
		copied := make([]*biz.Device, 0, len(devices))
		for _, device := range devices {
			clone := *device
			copied = append(copied, &clone)
		}
		cloned[key] = copied
	}
	return cloned
}

func cloneProfileMapByAccountKey(source map[string][]*biz.Profile) map[string][]*biz.Profile {
	cloned := make(map[string][]*biz.Profile, len(source))
	for key, profiles := range source {
		copied := make([]*biz.Profile, 0, len(profiles))
		for _, profile := range profiles {
			clone := *profile
			copied = append(copied, &clone)
		}
		cloned[key] = copied
	}
	return cloned
}

func cloneProfileMapByBindingRef(source map[string][]*biz.Profile) map[string][]*biz.Profile {
	cloned := make(map[string][]*biz.Profile, len(source))
	for key, profiles := range source {
		copied := make([]*biz.Profile, 0, len(profiles))
		for _, profile := range profiles {
			clone := *profile
			copied = append(copied, &clone)
		}
		cloned[key] = copied
	}
	return cloned
}

func cloneArtifactMap(source map[string]*biz.Artifact) map[string]*biz.Artifact {
	cloned := make(map[string]*biz.Artifact, len(source))
	for key, artifact := range source {
		clone := *artifact
		cloned[key] = &clone
	}
	return cloned
}
