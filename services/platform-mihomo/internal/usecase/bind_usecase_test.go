package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "platform-mihomo-service/api/mihomo/v1"
	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

func TestBindCredentialPersistsCredentialDeviceAndProfiles(t *testing.T) {
	uc := newBindUsecaseForTest()

	resp, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(101), resp.BindingID)
	require.Equal(t, "binding_101_10001", resp.PlatformAccountID)
	require.Equal(t, v1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE, resp.Status)
	require.Len(t, resp.Profiles, 1)
	require.Equal(t, "1008611", resp.Profiles[0].PlayerId)
	require.True(t, resp.Profiles[0].IsDefault)

	credential := uc.credentialRepo.byPlatformAccountID[resp.PlatformAccountID]
	require.NotNil(t, credential)
	require.Equal(t, uint64(101), credential.BindingID)
	require.Equal(t, "mihomo", credential.Platform)
	require.Equal(t, "10001", credential.AccountID)
	require.Equal(t, "cn_gf01", credential.Region)
	require.Equal(t, "v1", credential.CredentialVersion)
	require.Equal(t, "active", credential.Status)
	require.NotEmpty(t, credential.CredentialBlob)
	require.NotNil(t, credential.LastValidatedAt)

	decrypted, err := internalcrypto.DecryptString(testEncryptionKey, credential.CredentialBlob)
	require.NoError(t, err)
	require.Equal(t, `{"account_id":"10001","cookie_token":"abc"}`, decrypted)

	devices := uc.deviceRepo.byPlatformAccountID[resp.PlatformAccountID]
	require.Len(t, devices, 1)
	require.Equal(t, uint64(101), devices[0].BindingID)
	require.Equal(t, "12345678-1234-1234-1234-123456789abc", devices[0].DeviceID)
	require.Equal(t, "abcdefghijklmn", devices[0].DeviceFP)
	require.NotNil(t, devices[0].DeviceName)
	require.Equal(t, "iPhone", *devices[0].DeviceName)
	require.True(t, devices[0].IsValid)
	require.NotNil(t, devices[0].LastSeenAt)

	persistedProfiles := uc.profileRepo.byPlatformAccountID[resp.PlatformAccountID]
	require.Len(t, persistedProfiles, 1)
	require.Equal(t, uint64(101), persistedProfiles[0].BindingID)
	require.Equal(t, "Traveler", persistedProfiles[0].Nickname)
	require.Equal(t, 60, persistedProfiles[0].Level)
	require.True(t, persistedProfiles[0].IsDefault)
	require.False(t, persistedProfiles[0].DiscoveredAt.IsZero())

	listed, err := uc.profileUsecase.ListProfiles(context.Background(), resp.PlatformAccountID)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "1008611", listed[0].PlayerId)

	primary, err := uc.profileUsecase.GetPrimaryProfile(context.Background(), resp.PlatformAccountID)
	require.NoError(t, err)
	require.NotNil(t, primary)
	require.Equal(t, "1008611", primary.PlayerId)
}

func TestBindCredentialPersistsBindingIDOnCredentialAndProfiles(t *testing.T) {
	uc := newBindUsecaseForTest()

	out, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(42), out.BindingID)

	credential, err := uc.credentialRepo.GetByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, uint64(42), credential.BindingID)
	require.Equal(t, out.PlatformAccountID, credential.PlatformAccountID)

	profiles, err := uc.profileRepo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotEmpty(t, profiles)
	require.Equal(t, uint64(42), profiles[0].BindingID)
	require.Equal(t, out.PlatformAccountID, profiles[0].PlatformAccountID)
}

func TestBindCredentialRejectsMissingBindingID(t *testing.T) {
	uc := newBindUsecaseForTest()

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.Error(t, err)
}

func TestFormatPlatformAccountIDUsesBindingPrefix(t *testing.T) {
	require.Equal(t, "binding_42_10001", FormatPlatformAccountID(42, "10001"))
}

func TestBindingIDFromPlatformAccountIDAcceptsLegacyPrefix(t *testing.T) {
	bindingID, err := BindingIDFromPlatformAccountID("hoyo_ref_42_10001")
	require.NoError(t, err)
	require.Equal(t, uint64(42), bindingID)
}

func TestBindCredentialRollsBackWhenProfileSaveFails(t *testing.T) {
	uc := newBindUsecaseForTest()
	uc.profileRepo.failSave = true

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.Error(t, err)
	require.Empty(t, uc.credentialRepo.byPlatformAccountID)
	require.Empty(t, uc.deviceRepo.byPlatformAccountID)
	require.Empty(t, uc.profileRepo.byPlatformAccountID)
}

func TestBindCredentialRebindRemovesOldPlatformScopedRows(t *testing.T) {
	client := &sequentialMihomoClient{
		results: []mihomoValidateResult{
			{
				accountID: "10001",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "1008611",
					Nickname: "Traveler",
					Level:    60,
				}},
			},
			{
				accountID: "20002",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "2008622",
					Nickname: "Rebound",
					Level:    55,
				}},
			},
		},
	}
	uc := newBindUsecaseForTestWithClient(client)

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	second, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)
	require.Equal(t, "binding_42_20002", second.PlatformAccountID)

	_, ok := uc.credentialRepo.byPlatformAccountID[first.PlatformAccountID]
	require.False(t, ok)
	require.NotContains(t, uc.deviceRepo.byPlatformAccountID, first.PlatformAccountID)
	require.NotContains(t, uc.profileRepo.byPlatformAccountID, first.PlatformAccountID)

	credential, err := uc.credentialRepo.GetByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, second.PlatformAccountID, credential.PlatformAccountID)

	profiles, err := uc.profileRepo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, second.PlatformAccountID, profiles[0].PlatformAccountID)
	require.Equal(t, "2008622", profiles[0].PlayerID)

	devices, err := uc.deviceRepo.ListByPlatformAccountID(context.Background(), second.PlatformAccountID)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "device-new", devices[0].DeviceID)
}

func TestBindCredentialRebindInvalidatesBindingScopedArtifacts(t *testing.T) {
	client := &sequentialMihomoClient{
		results: []mihomoValidateResult{
			{
				accountID: "10001",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "1008611",
					Nickname: "Traveler",
					Level:    60,
				}},
			},
			{
				accountID: "20002",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "1008611",
					Nickname: "Rebound",
					Level:    55,
				}},
			},
		},
	}
	uc := newBindUsecaseForTestWithClient(client)
	uc.artifactRepo = newMemoryArtifactRepo()
	uc.BindUsecase.artifactRepo = uc.artifactRepo

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingID:         42,
		PlatformAccountID: first.PlatformAccountID,
		ArtifactType:      authKeyArtifactType,
		ArtifactValue:     "stale-authkey",
		ScopeKey:          "1008611",
		ExpiresAt:         time.Now().UTC().Add(time.Hour),
	}))

	_, err = uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)

	artifact, err := uc.artifactRepo.GetByBindingID(context.Background(), 42, authKeyArtifactType, "1008611")
	require.NoError(t, err)
	require.Nil(t, artifact)
}

func TestBindCredentialFreshBindInvalidatesOrphanBindingScopedArtifacts(t *testing.T) {
	uc := newBindUsecaseForTest()
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingID:         42,
		PlatformAccountID: "binding_42_old",
		ArtifactType:      authKeyArtifactType,
		ArtifactValue:     "orphan-authkey",
		ScopeKey:          "1008611",
		ExpiresAt:         time.Now().UTC().Add(time.Hour),
	}))

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)

	artifact, err := uc.artifactRepo.GetByBindingID(context.Background(), 42, authKeyArtifactType, "1008611")
	require.NoError(t, err)
	require.Nil(t, artifact)
}

func TestBindCredentialRollbackRestoresArtifactsDeletedDuringTransaction(t *testing.T) {
	uc := newBindUsecaseForTest()
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingID:         42,
		PlatformAccountID: "binding_42_old",
		ArtifactType:      authKeyArtifactType,
		ArtifactValue:     "rollback-authkey",
		ScopeKey:          "1008611",
		ExpiresAt:         time.Now().UTC().Add(time.Hour),
	}))
	uc.profileRepo.failSave = true

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.Error(t, err)

	artifact, err := uc.artifactRepo.GetByBindingID(context.Background(), 42, authKeyArtifactType, "1008611")
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, "rollback-authkey", artifact.ArtifactValue)
}

func TestBindCredentialRebindRemovesOldDefaultProfileBeforeSavingNewDefault(t *testing.T) {
	client := &sequentialMihomoClient{
		results: []mihomoValidateResult{
			{
				accountID: "10001",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "1008611",
					Nickname: "Traveler",
					Level:    60,
				}},
			},
			{
				accountID: "20002",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "2008622",
					Nickname: "Rebound",
					Level:    55,
				}},
			},
		},
	}
	uc := newBindUsecaseForTestWithClient(client)

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	second, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)
	require.Equal(t, "binding_42_20002", second.PlatformAccountID)
	require.NotContains(t, uc.profileRepo.byPlatformAccountID, first.PlatformAccountID)

	profiles, err := uc.profileRepo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.True(t, profiles[0].IsDefault)
	require.Equal(t, second.PlatformAccountID, profiles[0].PlatformAccountID)
}

func TestBindCredentialRebindRollbackRestoresPreviousBindingState(t *testing.T) {
	client := &sequentialMihomoClient{
		results: []mihomoValidateResult{
			{
				accountID: "10001",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "1008611",
					Nickname: "Traveler",
					Level:    60,
				}},
			},
			{
				accountID: "20002",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "2008622",
					Nickname: "Rebound",
					Level:    55,
				}},
			},
		},
	}
	uc := newBindUsecaseForTestWithClient(client)

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	uc.profileRepo.failSave = true
	_, err = uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.Error(t, err)

	credential, err := uc.credentialRepo.GetByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, first.PlatformAccountID, credential.PlatformAccountID)

	devices, err := uc.deviceRepo.ListByPlatformAccountID(context.Background(), first.PlatformAccountID)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "device-old", devices[0].DeviceID)
	require.NotContains(t, uc.deviceRepo.byPlatformAccountID, "binding_42_20002")

	profiles, err := uc.profileRepo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, first.PlatformAccountID, profiles[0].PlatformAccountID)
	require.Equal(t, "1008611", profiles[0].PlayerID)
	require.NotContains(t, uc.profileRepo.byPlatformAccountID, "binding_42_20002")
}

func TestBindCredentialRebindRollbackRestoresPreviousRowsWhenCleanupFails(t *testing.T) {
	client := &sequentialMihomoClient{
		results: []mihomoValidateResult{
			{
				accountID: "10001",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "1008611",
					Nickname: "Traveler",
					Level:    60,
				}},
			},
			{
				accountID: "20002",
				region:    "cn_gf01",
				profiles: []platformmihomo.DiscoveredProfile{{
					GameBiz:  "hk4e_cn",
					Region:   "cn_gf01",
					PlayerID: "2008622",
					Nickname: "Rebound",
					Level:    55,
				}},
			},
		},
	}
	uc := newBindUsecaseForTestWithClient(client)

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	uc.deviceRepo.failDeleteByPlatformAccountID[first.PlatformAccountID] = errors.New("cleanup failed")
	_, err = uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.ErrorContains(t, err, "cleanup failed")

	credential, err := uc.credentialRepo.GetByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, first.PlatformAccountID, credential.PlatformAccountID)

	devices, err := uc.deviceRepo.ListByPlatformAccountID(context.Background(), first.PlatformAccountID)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "device-old", devices[0].DeviceID)
	require.NotContains(t, uc.deviceRepo.byPlatformAccountID, "binding_42_20002")

	profiles, err := uc.profileRepo.ListByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, first.PlatformAccountID, profiles[0].PlatformAccountID)
	require.Equal(t, "1008611", profiles[0].PlayerID)
	require.NotContains(t, uc.profileRepo.byPlatformAccountID, "binding_42_20002")
}

func TestBindCredentialRebindUsesLatestBindingSnapshotInsideTransaction(t *testing.T) {
	uc := newBindUsecaseForTest()

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	mutatedAccountID := FormatPlatformAccountID(42, "30003")
	uc.client = &mutatingMihomoClient{
		repo:           uc.credentialRepo,
		deviceRepo:     uc.deviceRepo,
		profileRepo:    uc.profileRepo,
		bindingID:      42,
		newAccountID:   mutatedAccountID,
		discoveredID:   "20002",
		discoveredUID:  "2008622",
		discoveredName: "Fresh",
	}

	second, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        42,
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)
	require.Equal(t, "binding_42_20002", second.PlatformAccountID)

	_, ok := uc.credentialRepo.byPlatformAccountID[mutatedAccountID]
	require.False(t, ok)
	require.NotContains(t, uc.deviceRepo.byPlatformAccountID, mutatedAccountID)
	require.NotContains(t, uc.profileRepo.byPlatformAccountID, mutatedAccountID)

	credential, err := uc.credentialRepo.GetByBindingID(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, second.PlatformAccountID, credential.PlatformAccountID)
	require.NotEqual(t, first.PlatformAccountID, credential.PlatformAccountID)
}

func TestBindCredentialValidatesBeforeTransaction(t *testing.T) {
	client := &transactionObservingClient{}
	uc := newBindUsecaseForTestWithClient(client)

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingID:        101,
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)
	require.False(t, client.calledDuringTransaction)
}

var testEncryptionKey = []byte("0123456789abcdef0123456789abcdef")

type bindUsecaseTestHarness struct {
	*BindUsecase
	credentialRepo *memoryCredentialRepo
	deviceRepo     *memoryDeviceRepo
	profileRepo    *memoryProfileRepo
	artifactRepo   *memoryArtifactRepo
	profileUsecase *ProfileUsecase
}

func newBindUsecaseForTest() *bindUsecaseTestHarness {
	return newBindUsecaseForTestWithClient(platformmihomo.StubClient{})
}

func newBindUsecaseForTestWithClient(client platformmihomo.Client) *bindUsecaseTestHarness {
	credentialRepo := newMemoryCredentialRepo()
	deviceRepo := newMemoryDeviceRepo()
	profileRepo := newMemoryProfileRepo()
	artifactRepo := newMemoryArtifactRepo()
	credentialRepo.deviceRepo = deviceRepo
	credentialRepo.profileRepo = profileRepo
	credentialRepo.artifactRepo = artifactRepo
	if observingClient, ok := client.(*transactionObservingClient); ok {
		observingClient.repo = credentialRepo
	}

	bindUsecase := NewBindUsecase(credentialRepo, deviceRepo, profileRepo, client, testEncryptionKey, artifactRepo)

	return &bindUsecaseTestHarness{
		BindUsecase:    bindUsecase,
		credentialRepo: credentialRepo,
		deviceRepo:     deviceRepo,
		profileRepo:    profileRepo,
		artifactRepo:   artifactRepo,
		profileUsecase: NewProfileUsecase(profileRepo),
	}
}

type mihomoValidateResult struct {
	accountID string
	region    string
	profiles  []platformmihomo.DiscoveredProfile
	err       error
}

type sequentialMihomoClient struct {
	results []mihomoValidateResult
	index   int
}

func (c *sequentialMihomoClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	result := c.results[c.index]
	c.index++
	return result.accountID, result.region, result.profiles, result.err
}

func (c *sequentialMihomoClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "stub-authkey", 300, nil
}

type mutatingMihomoClient struct {
	repo           *memoryCredentialRepo
	deviceRepo     *memoryDeviceRepo
	profileRepo    *memoryProfileRepo
	bindingID      uint64
	newAccountID   string
	discoveredID   string
	discoveredUID  string
	discoveredName string
	mutated        bool
}

func (c *mutatingMihomoClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	if !c.mutated {
		credential := &biz.Credential{
			BindingID:         c.bindingID,
			PlatformAccountID: c.newAccountID,
			Platform:          "mihomo",
			AccountID:         "30003",
			Region:            "cn_gf01",
			CredentialBlob:    "blob-30003",
			CredentialVersion: "v1",
			Status:            "active",
		}
		_ = c.repo.Save(context.Background(), credential)
		_ = c.deviceRepo.Save(context.Background(), &biz.Device{BindingID: c.bindingID, PlatformAccountID: c.newAccountID, DeviceID: "device-mutated", DeviceFP: "fp-mutated", IsValid: true})
		_ = c.profileRepo.DeleteMissingByBindingID(context.Background(), c.bindingID, nil)
		_ = c.profileRepo.Save(context.Background(), &biz.Profile{BindingID: c.bindingID, PlatformAccountID: c.newAccountID, GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "3008611", Nickname: "Mutated", Level: 60, IsDefault: true})
		c.mutated = true
	}

	return c.discoveredID, "cn_gf01", []platformmihomo.DiscoveredProfile{{
		GameBiz:  "hk4e_cn",
		Region:   "cn_gf01",
		PlayerID: c.discoveredUID,
		Nickname: c.discoveredName,
		Level:    55,
	}}, nil
}

func (c *mutatingMihomoClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "stub-authkey", 300, nil
}

type transactionObservingClient struct {
	calledDuringTransaction bool
	repo                    *memoryCredentialRepo
}

func (c *transactionObservingClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	c.calledDuringTransaction = c.repo.inTransaction
	return "10001", "cn_gf01", []platformmihomo.DiscoveredProfile{{
		GameBiz:  "hk4e_cn",
		Region:   "cn_gf01",
		PlayerID: "1008611",
		Nickname: "Traveler",
		Level:    60,
	}}, nil
}

func (c *transactionObservingClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "stub-authkey", 300, nil
}

type memoryCredentialRepo struct {
	byPlatformAccountID map[string]*biz.Credential
	byBindingID         map[uint64]*biz.Credential
	deviceRepo          *memoryDeviceRepo
	profileRepo         *memoryProfileRepo
	artifactRepo        *memoryArtifactRepo
	inTransaction       bool
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

func (r *memoryCredentialRepo) GetByPlatformAccountID(_ context.Context, platformAccountID string) (*biz.Credential, error) {
	credential := r.byPlatformAccountID[platformAccountID]
	if credential == nil {
		return nil, nil
	}
	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) GetByBindingID(_ context.Context, bindingID uint64) (*biz.Credential, error) {
	credential := r.byBindingID[bindingID]
	if credential == nil {
		return nil, nil
	}
	clone := *credential
	return &clone, nil
}

func (r *memoryCredentialRepo) DeleteByPlatformAccountID(_ context.Context, platformAccountID string) error {
	if credential := r.byPlatformAccountID[platformAccountID]; credential != nil {
		if current := r.byBindingID[credential.BindingID]; current != nil && current.PlatformAccountID == platformAccountID {
			delete(r.byBindingID, credential.BindingID)
		}
	}
	delete(r.byPlatformAccountID, platformAccountID)
	return nil
}

func (r *memoryCredentialRepo) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	credentialByPlatform := cloneCredentialMapByPlatformAccountID(r.byPlatformAccountID)
	credentialByBinding := cloneCredentialMapByBindingID(r.byBindingID)
	deviceByPlatform := cloneDeviceMapByPlatformAccountID(r.deviceRepo.byPlatformAccountID)
	deviceByBinding := cloneDeviceMapByBindingID(r.deviceRepo.byBindingID)
	profileByPlatform := cloneProfileMapByPlatformAccountID(r.profileRepo.byPlatformAccountID)
	profileByBinding := cloneProfileMapByBindingID(r.profileRepo.byBindingID)
	artifactByKey := cloneArtifactMap(r.artifactRepo.artifacts)
	r.inTransaction = true
	defer func() {
		r.inTransaction = false
	}()
	if err := fn(ctx); err != nil {
		r.byPlatformAccountID = credentialByPlatform
		r.byBindingID = credentialByBinding
		r.deviceRepo.byPlatformAccountID = deviceByPlatform
		r.deviceRepo.byBindingID = deviceByBinding
		r.profileRepo.byPlatformAccountID = profileByPlatform
		r.profileRepo.byBindingID = profileByBinding
		r.artifactRepo.artifacts = artifactByKey
		return err
	}
	return nil
}

type memoryDeviceRepo struct {
	byPlatformAccountID           map[string][]*biz.Device
	byBindingID                   map[uint64][]*biz.Device
	failDeleteByPlatformAccountID map[string]error
}

func newMemoryDeviceRepo() *memoryDeviceRepo {
	return &memoryDeviceRepo{
		byPlatformAccountID:           make(map[string][]*biz.Device),
		byBindingID:                   make(map[uint64][]*biz.Device),
		failDeleteByPlatformAccountID: make(map[string]error),
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
	if err := r.failDeleteByPlatformAccountID[platformAccountID]; err != nil {
		return err
	}
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
	failSave            bool
}

func newMemoryProfileRepo() *memoryProfileRepo {
	return &memoryProfileRepo{
		byPlatformAccountID: make(map[string][]*biz.Profile),
		byBindingID:         make(map[uint64][]*biz.Profile),
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

func (r *memoryProfileRepo) hasConflictingDefault(profile *biz.Profile) bool {
	for _, existing := range r.byBindingID[profile.BindingID] {
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

func (r *memoryProfileRepo) ListByBindingID(_ context.Context, bindingID uint64) ([]*biz.Profile, error) {
	profiles := r.byBindingID[bindingID]
	result := make([]*biz.Profile, 0, len(profiles))
	for _, profile := range profiles {
		clone := *profile
		result = append(result, &clone)
	}
	return result, nil
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
	if len(filtered) == 0 {
		if len(profiles) > 0 {
			delete(r.byBindingID, profiles[0].BindingID)
		}
		return nil
	}
	r.byBindingID[filtered[0].BindingID] = filtered
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

var _ biz.CredentialRepository = (*memoryCredentialRepo)(nil)
var _ biz.DeviceRepository = (*memoryDeviceRepo)(nil)
var _ biz.ProfileRepository = (*memoryProfileRepo)(nil)

func cloneCredentialMapByPlatformAccountID(source map[string]*biz.Credential) map[string]*biz.Credential {
	cloned := make(map[string]*biz.Credential, len(source))
	for key, credential := range source {
		clone := *credential
		cloned[key] = &clone
	}
	return cloned
}

func cloneCredentialMapByBindingID(source map[uint64]*biz.Credential) map[uint64]*biz.Credential {
	cloned := make(map[uint64]*biz.Credential, len(source))
	for key, credential := range source {
		clone := *credential
		cloned[key] = &clone
	}
	return cloned
}

func cloneDeviceMapByPlatformAccountID(source map[string][]*biz.Device) map[string][]*biz.Device {
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

func cloneDeviceMapByBindingID(source map[uint64][]*biz.Device) map[uint64][]*biz.Device {
	cloned := make(map[uint64][]*biz.Device, len(source))
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

func cloneProfileMapByPlatformAccountID(source map[string][]*biz.Profile) map[string][]*biz.Profile {
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

func cloneProfileMapByBindingID(source map[uint64][]*biz.Profile) map[uint64][]*biz.Profile {
	cloned := make(map[uint64][]*biz.Profile, len(source))
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
