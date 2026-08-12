package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	mihomostub "platform-mihomo-service/internal/testkit/mihomostub"
)

func TestBindCredentialPersistsCredentialDeviceAndProfiles(t *testing.T) {
	uc := newBindUsecaseForTest()

	resp, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-101",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.NoError(t, err)
	require.Equal(t, "binding-101", resp.BindingRef)
	require.Equal(t, FormatAccountKey("10001"), resp.AccountKey)
	require.Equal(t, CredentialStatusActive, resp.Status)
	require.Len(t, resp.Profiles, 1)
	require.Equal(t, "1008611", resp.Profiles[0].PlayerID)
	require.True(t, resp.Profiles[0].IsDefault)

	credential := uc.credentialRepo.byAccountKey[resp.AccountKey]
	require.NotNil(t, credential)
	require.Equal(t, "binding-101", credential.BindingRef)
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

	devices := uc.deviceRepo.byAccountKey[resp.AccountKey]
	require.Len(t, devices, 1)
	require.Equal(t, "binding-101", devices[0].BindingRef)
	require.Equal(t, "12345678-1234-1234-1234-123456789abc", devices[0].DeviceID)
	require.Equal(t, "abcdefghijklmn", devices[0].DeviceFP)
	require.NotNil(t, devices[0].DeviceName)
	require.Equal(t, "iPhone", *devices[0].DeviceName)
	require.True(t, devices[0].IsValid)
	require.NotNil(t, devices[0].LastSeenAt)

	persistedProfiles := uc.profileRepo.byAccountKey[resp.AccountKey]
	require.Len(t, persistedProfiles, 1)
	require.Equal(t, "binding-101", persistedProfiles[0].BindingRef)
	require.Equal(t, "Traveler", persistedProfiles[0].Nickname)
	require.Equal(t, 60, persistedProfiles[0].Level)
	require.True(t, persistedProfiles[0].IsDefault)
	require.False(t, persistedProfiles[0].DiscoveredAt.IsZero())

	listed, err := uc.profileUsecase.ListProfiles(context.Background(), resp.AccountKey)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "1008611", listed[0].PlayerID)

	primary, err := uc.profileUsecase.GetPrimaryProfile(context.Background(), resp.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, primary)
	require.Equal(t, "1008611", primary.PlayerID)
}

func TestBindCredentialPersistsBindingRefOnCredentialAndProfiles(t *testing.T) {
	uc := newBindUsecaseForTest()

	out, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.NoError(t, err)
	require.Equal(t, "binding-42", out.BindingRef)

	credential, err := uc.credentialRepo.GetByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, "binding-42", credential.BindingRef)
	require.Equal(t, out.AccountKey, credential.AccountKey)

	profiles, err := uc.profileRepo.ListByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.NotEmpty(t, profiles)
	require.Equal(t, "binding-42", profiles[0].BindingRef)
	require.Equal(t, out.AccountKey, profiles[0].AccountKey)
}

func TestBindCredentialRejectsMissingBindingRef(t *testing.T) {
	uc := newBindUsecaseForTest()

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
	})
	require.Error(t, err)
}

func TestFormatAccountKeyIsStableAcrossBindings(t *testing.T) {
	first := FormatAccountKey("10001")
	second := FormatAccountKey("10001")

	require.Equal(t, first, second)
	require.NotContains(t, first, "10001")
	require.NotContains(t, first, "binding")
}

func TestFormatProfileRefIsStableForProfileIdentity(t *testing.T) {
	accountKey := FormatAccountKey("10001")
	first := FormatProfileRef(accountKey, "hk4e_cn", "cn_gf01", "1008611")
	second := FormatProfileRef(accountKey, "hk4e_cn", "cn_gf01", "1008611")

	require.Equal(t, first, second)
	require.NotEqual(t, first, FormatProfileRef(accountKey, "hk4e_cn", "cn_gf01", "2008611"))
	require.NotContains(t, first, "1008611")
}

func TestBindCredentialRollsBackWhenProfileSaveFails(t *testing.T) {
	uc := newBindUsecaseForTest()
	uc.profileRepo.failSave = true

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-101",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "12345678-1234-1234-1234-123456789abc",
		DeviceFP:         "abcdefghijklmn",
		DeviceName:       "iPhone",
	})
	require.Error(t, err)
	require.Empty(t, uc.credentialRepo.byAccountKey)
	require.Empty(t, uc.deviceRepo.byAccountKey)
	require.Empty(t, uc.profileRepo.byAccountKey)
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
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	second, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)
	require.Equal(t, FormatAccountKey("20002"), second.AccountKey)

	_, ok := uc.credentialRepo.byAccountKey[first.AccountKey]
	require.False(t, ok)
	require.NotContains(t, uc.deviceRepo.byAccountKey, first.AccountKey)
	require.NotContains(t, uc.profileRepo.byAccountKey, first.AccountKey)

	credential, err := uc.credentialRepo.GetByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, second.AccountKey, credential.AccountKey)

	profiles, err := uc.profileRepo.ListByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, second.AccountKey, profiles[0].AccountKey)
	require.Equal(t, "2008622", profiles[0].PlayerID)

	devices, err := uc.deviceRepo.ListByAccountKey(context.Background(), second.AccountKey)
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
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-42",
		AccountKey:    first.AccountKey,
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: "stale-authkey",
		ScopeKey:      "1008611",
		ExpiresAt:     time.Now().UTC().Add(time.Hour),
	}))

	_, err = uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)

	artifact, err := uc.artifactRepo.GetByBindingRef(context.Background(), "binding-42", authKeyArtifactType, "1008611")
	require.NoError(t, err)
	require.Nil(t, artifact)
}

func TestBindCredentialFreshBindInvalidatesOrphanBindingScopedArtifacts(t *testing.T) {
	uc := newBindUsecaseForTest()
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-42",
		AccountKey:    "binding_42_old",
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: "orphan-authkey",
		ScopeKey:      "1008611",
		ExpiresAt:     time.Now().UTC().Add(time.Hour),
	}))

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)

	artifact, err := uc.artifactRepo.GetByBindingRef(context.Background(), "binding-42", authKeyArtifactType, "1008611")
	require.NoError(t, err)
	require.Nil(t, artifact)
}

func TestBindCredentialRollbackRestoresArtifactsDeletedDuringTransaction(t *testing.T) {
	uc := newBindUsecaseForTest()
	require.NoError(t, uc.artifactRepo.Put(context.Background(), &biz.Artifact{
		BindingRef:    "binding-42",
		AccountKey:    "binding_42_old",
		ArtifactType:  authKeyArtifactType,
		ArtifactValue: "rollback-authkey",
		ScopeKey:      "1008611",
		ExpiresAt:     time.Now().UTC().Add(time.Hour),
	}))
	uc.profileRepo.failSave = true

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.Error(t, err)

	artifact, err := uc.artifactRepo.GetByBindingRef(context.Background(), "binding-42", authKeyArtifactType, "1008611")
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
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	second, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)
	require.Equal(t, FormatAccountKey("20002"), second.AccountKey)
	require.NotContains(t, uc.profileRepo.byAccountKey, first.AccountKey)

	profiles, err := uc.profileRepo.ListByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.True(t, profiles[0].IsDefault)
	require.Equal(t, second.AccountKey, profiles[0].AccountKey)
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
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	uc.profileRepo.failSave = true
	_, err = uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.Error(t, err)

	credential, err := uc.credentialRepo.GetByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, first.AccountKey, credential.AccountKey)

	devices, err := uc.deviceRepo.ListByAccountKey(context.Background(), first.AccountKey)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "device-old", devices[0].DeviceID)
	require.NotContains(t, uc.deviceRepo.byAccountKey, "binding_42_20002")

	profiles, err := uc.profileRepo.ListByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, first.AccountKey, profiles[0].AccountKey)
	require.Equal(t, "1008611", profiles[0].PlayerID)
	require.NotContains(t, uc.profileRepo.byAccountKey, "binding_42_20002")
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
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	uc.deviceRepo.failDeleteByAccountKey[first.AccountKey] = errors.New("cleanup failed")
	_, err = uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.ErrorContains(t, err, "cleanup failed")

	credential, err := uc.credentialRepo.GetByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, first.AccountKey, credential.AccountKey)

	devices, err := uc.deviceRepo.ListByAccountKey(context.Background(), first.AccountKey)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, "device-old", devices[0].DeviceID)
	require.NotContains(t, uc.deviceRepo.byAccountKey, "binding_42_20002")

	profiles, err := uc.profileRepo.ListByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Equal(t, first.AccountKey, profiles[0].AccountKey)
	require.Equal(t, "1008611", profiles[0].PlayerID)
	require.NotContains(t, uc.profileRepo.byAccountKey, "binding_42_20002")
}

func TestBindCredentialRebindUsesLatestBindingSnapshotInsideTransaction(t *testing.T) {
	uc := newBindUsecaseForTest()

	first, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"10001","cookie_token":"abc"}`,
		DeviceID:         "device-old",
		DeviceFP:         "fp-old",
	})
	require.NoError(t, err)

	mutatedAccountID := FormatAccountKey("30003")
	uc.client = &mutatingMihomoClient{
		repo:           uc.credentialRepo,
		deviceRepo:     uc.deviceRepo,
		profileRepo:    uc.profileRepo,
		bindingRef:     "binding-42",
		newAccountID:   mutatedAccountID,
		discoveredID:   "20002",
		discoveredUID:  "2008622",
		discoveredName: "Fresh",
	}

	second, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-42",
		CookieBundleJSON: `{"account_id":"20002","cookie_token":"def"}`,
		DeviceID:         "device-new",
		DeviceFP:         "fp-new",
	})
	require.NoError(t, err)
	require.Equal(t, FormatAccountKey("20002"), second.AccountKey)

	_, ok := uc.credentialRepo.byAccountKey[mutatedAccountID]
	require.False(t, ok)
	require.NotContains(t, uc.deviceRepo.byAccountKey, mutatedAccountID)
	require.NotContains(t, uc.profileRepo.byAccountKey, mutatedAccountID)

	credential, err := uc.credentialRepo.GetByBindingRef(context.Background(), "binding-42")
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, second.AccountKey, credential.AccountKey)
	require.NotEqual(t, first.AccountKey, credential.AccountKey)
}

func TestBindCredentialValidatesBeforeTransaction(t *testing.T) {
	client := &transactionObservingClient{}
	uc := newBindUsecaseForTestWithClient(client)

	_, err := uc.BindCredential(context.Background(), BindCredentialInput{
		BindingRef:       "binding-101",
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
	return newBindUsecaseForTestWithClient(mihomostub.Client{})
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
	bindingRef     string
	newAccountID   string
	discoveredID   string
	discoveredUID  string
	discoveredName string
	mutated        bool
}

func (c *mutatingMihomoClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	if !c.mutated {
		credential := &biz.Credential{
			BindingRef:        c.bindingRef,
			AccountKey:        c.newAccountID,
			Platform:          "mihomo",
			AccountID:         "30003",
			Region:            "cn_gf01",
			CredentialBlob:    "blob-30003",
			CredentialVersion: "v1",
			Status:            "active",
		}
		_ = c.repo.Save(context.Background(), credential)
		_ = c.deviceRepo.Save(context.Background(), &biz.Device{BindingRef: c.bindingRef, AccountKey: c.newAccountID, DeviceID: "device-mutated", DeviceFP: "fp-mutated", IsValid: true})
		_ = c.profileRepo.DeleteMissingByBindingRef(context.Background(), c.bindingRef, nil)
		_ = c.profileRepo.Save(context.Background(), &biz.Profile{BindingRef: c.bindingRef, AccountKey: c.newAccountID, GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerID: "3008611", Nickname: "Mutated", Level: 60, IsDefault: true})
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
