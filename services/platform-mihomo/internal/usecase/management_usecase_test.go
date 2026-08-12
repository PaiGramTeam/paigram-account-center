package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

func TestGetCredentialSummaryReturnsDevicesAndProfiles(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	summary, err := uc.GetCredentialSummary(context.Background(), accountKey)
	require.NoError(t, err)
	require.Equal(t, accountKey, summary.AccountKey)
	require.Equal(t, CredentialStatusActive, summary.Status)
	require.NotNil(t, summary.LastValidatedAt)
	require.NotEmpty(t, summary.Profiles)
	require.NotEmpty(t, summary.Devices)
	require.Equal(t, "12345678-1234-1234-1234-123456789abc", summary.Devices[0].DeviceID)
	require.Equal(t, "1008611", summary.Profiles[0].PlayerID)
}

func TestUpdateCredentialReturnsUpdatedSummary(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	summary, err := uc.UpdateCredential(context.Background(), UpdateCredentialInput{
		AccountKey: accountKey,
		BindCredentialInput: BindCredentialInput{
			BindingRef:       "binding-101",
			CookieBundleJSON: `{"account_id":"10001","cookie_token":"updated"}`,
			DeviceID:         "device-2",
			DeviceFP:         "fp-2",
			DeviceName:       "iPad",
			RegionHint:       "cn_gf01",
		},
	})
	require.NoError(t, err)
	require.Equal(t, accountKey, summary.AccountKey)
	require.NotEmpty(t, summary.Devices)
	require.True(t, containsDevice(summary.Devices, "device-2"))
	require.NotEmpty(t, summary.Profiles)
}

func TestUpdateCredentialRejectsPlatformAccountMismatch(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)
	uc.bindUC = NewBindUsecase(uc.credentialRepo, uc.deviceRepo, uc.profileRepo, mismatchClient{}, testEncryptionKey, uc.artifactRepo)
	originalCredential, err := uc.credentialRepo.GetByAccountKey(context.Background(), accountKey)
	require.NoError(t, err)
	originalDevices, err := uc.deviceRepo.ListByAccountKey(context.Background(), accountKey)
	require.NoError(t, err)
	originalProfiles, err := uc.profileRepo.ListByAccountKey(context.Background(), accountKey)
	require.NoError(t, err)

	_, err = uc.UpdateCredential(context.Background(), UpdateCredentialInput{
		AccountKey: accountKey,
		BindCredentialInput: BindCredentialInput{
			BindingRef:       "binding-101",
			CookieBundleJSON: `{"account_id":"20002","cookie_token":"updated"}`,
			DeviceID:         "device-3",
			DeviceFP:         "fp-3",
			DeviceName:       "iPad",
		},
	})
	require.ErrorIs(t, err, ErrPlatformAccountMismatch)
	require.Zero(t, uc.managementRepo.deleteCalls)
	currentCredential, err := uc.credentialRepo.GetByAccountKey(context.Background(), accountKey)
	require.NoError(t, err)
	require.NotNil(t, currentCredential)
	require.Equal(t, originalCredential.AccountKey, currentCredential.AccountKey)
	require.Equal(t, originalCredential.AccountID, currentCredential.AccountID)
	currentDevices, err := uc.deviceRepo.ListByAccountKey(context.Background(), accountKey)
	require.NoError(t, err)
	require.Len(t, currentDevices, len(originalDevices))
	require.Equal(t, originalDevices[0].DeviceID, currentDevices[0].DeviceID)
	currentProfiles, err := uc.profileRepo.ListByAccountKey(context.Background(), accountKey)
	require.NoError(t, err)
	require.Len(t, currentProfiles, len(originalProfiles))
	require.Equal(t, originalProfiles[0].PlayerID, currentProfiles[0].PlayerID)
	require.Nil(t, uc.credentialRepo.byAccountKey["binding_101_20002"])
}

func TestUpdateCredentialRemovesStaleProfiles(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)
	uc.bindUC = NewBindUsecase(uc.credentialRepo, uc.deviceRepo, uc.profileRepo, profileSwitchClient{}, testEncryptionKey, uc.artifactRepo)

	summary, err := uc.UpdateCredential(context.Background(), UpdateCredentialInput{
		AccountKey: accountKey,
		BindCredentialInput: BindCredentialInput{
			BindingRef:       "binding-101",
			CookieBundleJSON: `{"account_id":"10001","cookie_token":"updated"}`,
			DeviceID:         "device-4",
			DeviceFP:         "fp-4",
			DeviceName:       "switch-device",
		},
	})
	require.NoError(t, err)
	require.Len(t, summary.Profiles, 1)
	require.Equal(t, "2008611", summary.Profiles[0].PlayerID)
}

func TestDeleteCredentialRemovesCredentialArtifactsAndRelations(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		AccountKey:    accountKey,
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: "authkey-1",
		ScopeKey:      "1008611",
		ExpiresAt:     time.Now().UTC().Add(5 * time.Minute),
	}))

	err := uc.DeleteCredential(context.Background(), accountKey)
	require.NoError(t, err)
	require.Empty(t, uc.credentialRepo.byAccountKey)
	require.Empty(t, uc.deviceRepo.byAccountKey)
	require.Empty(t, uc.profileRepo.byAccountKey)
	require.Empty(t, uc.artifactRepo.artifacts)

	_, err = uc.GetCredentialSummary(context.Background(), accountKey)
	require.ErrorIs(t, err, ErrCredentialNotFound)
}

func TestGetCredentialSummaryWithScopeRejectsForeignBinding(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	_, err := uc.GetCredentialSummaryWithScope(context.Background(), ScopeGuard{BindingRef: "binding-999", AccountKey: accountKey}, accountKey)
	require.ErrorIs(t, err, ErrBindingScopeDenied)
}

func TestDeleteCredentialWithScopeRejectsForeignBinding(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	err := uc.DeleteCredentialWithScope(context.Background(), ScopeGuard{BindingRef: "binding-999", AccountKey: accountKey}, accountKey)
	require.ErrorIs(t, err, ErrBindingScopeDenied)
}

func TestDeleteCredentialWithScopeDeletesCredentialGraphByBinding(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		AccountKey:    accountKey,
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: "authkey-1",
		ScopeKey:      "1008611",
		ExpiresAt:     time.Now().UTC().Add(5 * time.Minute),
	}))

	err := uc.DeleteCredentialWithScope(context.Background(), ScopeGuard{BindingRef: "binding-101", AccountKey: accountKey}, accountKey)
	require.NoError(t, err)
	require.Equal(t, 1, uc.managementRepo.deleteByBindingCalls)
	require.Zero(t, uc.managementRepo.deleteCalls)
	require.Nil(t, uc.credentialRepo.byBindingRef["binding-101"])
	require.NotContains(t, uc.deviceRepo.byBindingRef, "binding-101")
	require.NotContains(t, uc.profileRepo.byBindingRef, "binding-101")
	require.Empty(t, uc.artifactRepo.artifacts)
}

func TestGetCredentialSummaryWithScopeRejectsProfileScopedTicket(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	_, err := uc.GetCredentialSummaryWithScope(context.Background(), ScopeGuard{BindingRef: "binding-101", AccountKey: accountKey, ProfileRef: "profile-1001"}, accountKey)
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestUpdateCredentialWithScopeRejectsProfileScopedTicket(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	_, err := uc.UpdateCredentialWithScope(context.Background(), ScopeGuard{BindingRef: "binding-101", AccountKey: accountKey, ProfileRef: "profile-1001"}, UpdateCredentialInput{
		AccountKey: accountKey,
		BindCredentialInput: BindCredentialInput{
			BindingRef:       "binding-101",
			CookieBundleJSON: `{"account_id":"10001","cookie_token":"updated"}`,
			DeviceID:         "device-2",
			DeviceFP:         "fp-2",
			DeviceName:       "iPad",
			RegionHint:       "cn_gf01",
		},
	})
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

func TestDeleteCredentialWithScopeRejectsProfileScopedTicket(t *testing.T) {
	uc := newManagementUsecaseForTest(t)
	accountKey := bindCredentialForManagementTest(t, uc)

	err := uc.DeleteCredentialWithScope(context.Background(), ScopeGuard{BindingRef: "binding-101", AccountKey: accountKey, ProfileRef: "profile-1001"}, accountKey)
	require.ErrorIs(t, err, ErrProfileScopeDenied)
}

type managementUsecaseTestHarness struct {
	*ManagementUsecase
	credentialRepo *memoryCredentialRepo
	deviceRepo     *memoryDeviceRepo
	profileRepo    *memoryProfileRepo
	artifactRepo   *memoryArtifactRepo
	managementRepo *memoryManagementRepo
}

func newManagementUsecaseForTest(t *testing.T) *managementUsecaseTestHarness {
	t.Helper()
	return newManagementUsecaseForTestWithClient(t, nil)
}

func newManagementUsecaseForTestWithClient(t *testing.T, client platformmihomo.Client) *managementUsecaseTestHarness {
	t.Helper()

	bindHarness := newBindUsecaseForTest()
	if client != nil {
		bindHarness.BindUsecase = NewBindUsecase(bindHarness.credentialRepo, bindHarness.deviceRepo, bindHarness.profileRepo, client, testEncryptionKey, bindHarness.artifactRepo)
	}
	artifactRepo := bindHarness.artifactRepo
	managementRepo := &memoryManagementRepo{
		credentialRepo: bindHarness.credentialRepo,
		deviceRepo:     bindHarness.deviceRepo,
		profileRepo:    bindHarness.profileRepo,
		artifactRepo:   artifactRepo,
	}
	managementUsecase := NewManagementUsecase(
		bindHarness.credentialRepo,
		bindHarness.deviceRepo,
		bindHarness.profileRepo,
		artifactRepo,
		managementRepo,
		bindHarness.BindUsecase,
		bindHarness.profileUsecase,
	)

	return &managementUsecaseTestHarness{
		ManagementUsecase: managementUsecase,
		credentialRepo:    bindHarness.credentialRepo,
		deviceRepo:        bindHarness.deviceRepo,
		profileRepo:       bindHarness.profileRepo,
		artifactRepo:      artifactRepo,
		managementRepo:    managementRepo,
	}
}

func bindCredentialForManagementTest(t *testing.T, uc *managementUsecaseTestHarness) string {
	t.Helper()

	resp, err := uc.bindUC.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-101",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)
	return resp.AccountKey
}

type memoryManagementRepo struct {
	credentialRepo       *memoryCredentialRepo
	deviceRepo           *memoryDeviceRepo
	profileRepo          *memoryProfileRepo
	artifactRepo         *memoryArtifactRepo
	deleteCalls          int
	deleteByBindingCalls int
}

func (r *memoryManagementRepo) DeleteCredentialGraph(_ context.Context, accountKey string) error {
	r.deleteCalls++
	requireDeleteArtifacts(r.artifactRepo, accountKey)
	_ = r.credentialRepo.DeleteByAccountKey(context.Background(), accountKey)
	_ = r.deviceRepo.DeleteByAccountKey(context.Background(), accountKey)
	_ = r.profileRepo.DeleteByAccountKey(context.Background(), accountKey)
	return nil
}

func (r *memoryManagementRepo) DeleteCredentialGraphByBindingRef(_ context.Context, bindingRef string) error {
	r.deleteByBindingCalls++
	credential := r.credentialRepo.byBindingRef[bindingRef]
	if credential != nil {
		requireDeleteArtifacts(r.artifactRepo, credential.AccountKey)
		delete(r.credentialRepo.byAccountKey, credential.AccountKey)
	}
	delete(r.credentialRepo.byBindingRef, bindingRef)
	_ = r.deviceRepo.DeleteByBindingRef(context.Background(), bindingRef)
	profiles := r.profileRepo.byBindingRef[bindingRef]
	for _, profile := range profiles {
		delete(r.profileRepo.byAccountKey, profile.AccountKey)
	}
	delete(r.profileRepo.byBindingRef, bindingRef)
	return nil
}

func requireDeleteArtifacts(repo *memoryArtifactRepo, accountKey string) {
	for key, artifact := range repo.artifacts {
		if artifact.AccountKey == accountKey {
			delete(repo.artifacts, key)
		}
	}
}

func containsDevice(devices []*biz.Device, deviceID string) bool {
	for _, device := range devices {
		if device.DeviceID == deviceID {
			return true
		}
	}
	return false
}

type mismatchClient struct{}

func (mismatchClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "20002", "cn_gf01", []platformmihomo.DiscoveredProfile{{GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "2008611", Nickname: "Mismatch", Level: 60}}, nil
}

func (mismatchClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "", 0, nil
}

type profileSwitchClient struct{}

func (profileSwitchClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "10001", "cn_gf01", []platformmihomo.DiscoveredProfile{{GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "2008611", Nickname: "Switched", Level: 50}}, nil
}

func (profileSwitchClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "", 0, nil
}
