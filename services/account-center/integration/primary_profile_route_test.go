//go:build integration

package integration

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func TestPrimaryProfileRouteReturnsAuthoritativeAssignment(t *testing.T) {
	stack := newIntegrationStack(t)
	ownerID, accessToken, _, _, _ := registerAndLogin(t, stack, fmt.Sprintf("primary-route-%d@example.com", time.Now().UnixNano()), "OwnerPass123!")
	binding := model.PlatformAccountBinding{
		OwnerUserID: ownerID, Platform: "mihomo", BindingRef: "binding-primary-route", Generation: 4,
		ExternalAccountKey: sql.NullString{String: "cn:owner-main", Valid: true}, PlatformServiceKey: "platform-mihomo-service",
		DisplayName: "Owner Main", Status: model.PlatformAccountBindingStatusActive, ProfileRevision: 1, ProfileObservedRevision: 1,
	}
	require.NoError(t, stack.DB.Create(&binding).Error)
	profiles := []model.PlatformAccountProfile{
		{BindingID: binding.ID, PlatformProfileKey: "mihomo:10001", ProfileRef: "mihomo:10001", GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerUID: "10001", Nickname: "Traveler", IsPrimary: true},
		{BindingID: binding.ID, PlatformProfileKey: "mihomo:10002", ProfileRef: "mihomo:10002", GameBiz: "hk4e_global", Region: "os_asia", PlayerUID: "10002", Nickname: "Lumine"},
	}
	require.NoError(t, stack.DB.Create(&profiles).Error)
	require.NoError(t, stack.DB.Model(&binding).Update("primary_profile_id", profiles[0].ID).Error)
	stub := &platformBindingRouteStub{summaryResponse: &routeCredentialSummary{
		PlatformAccountId: "cn:owner-main", Status: platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
		Profiles: []*routeProfileSummary{
			{Id: 10001, PlatformAccountId: "cn:owner-main", GameBiz: "hk4e_cn", Region: "cn_gf01", PlayerId: "10001", Nickname: "Traveler", IsDefault: true},
			{Id: 10002, PlatformAccountId: "cn:owner-main", GameBiz: "hk4e_global", Region: "os_asia", PlayerId: "10002", Nickname: "Lumine"},
		},
	}}
	seedEnabledPlatformService(t, stack, startPlatformBindingRouteServer(t, stack, stub))

	response := performJSONRequest(t, stack.Router, http.MethodPatch, fmt.Sprintf("/api/v1/me/platform-accounts/%d/primary-profile", binding.ID), map[string]any{
		"profile_id": profiles[1].ID,
	}, authHeaders(accessToken))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	data := decodeResponseData(t, response)
	assert.Equal(t, float64(profiles[1].ID), data["primary_profile_id"])
	require.NotNil(t, stub.lastPrimary)
	assert.Equal(t, "mihomo:10002", stub.lastPrimary.GetProfileRef())
	assert.Equal(t, uint64(1), stub.lastPrimary.GetExpectedProfileRevision())

	invalid := performJSONRequest(t, stack.Router, http.MethodPatch, fmt.Sprintf("/api/v1/me/platform-accounts/%d/primary-profile", binding.ID), map[string]any{
		"profile_id": uint64(99999999),
	}, authHeaders(accessToken))
	require.Equal(t, http.StatusUnprocessableEntity, invalid.Code, invalid.Body.String())
}
