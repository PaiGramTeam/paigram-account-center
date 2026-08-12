//go:build integration

package integration

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"paigram/internal/model"
)

func TestConsumerGrantMutationReturnsAcceptedWhileFencePropagationIsPending(t *testing.T) {
	stack := newIntegrationStack(t)
	ownerID, accessToken, _, _, _ := registerAndLogin(t, stack, fmt.Sprintf("grant-propagation-%d@example.com", time.Now().UnixNano()), "OwnerPass123!")
	binding := model.PlatformAccountBinding{
		OwnerUserID: ownerID, Platform: "mihomo", ExternalAccountKey: sql.NullString{String: "account-grant-pending", Valid: true},
		PlatformServiceKey: "platform-mihomo-service", DisplayName: "Grant propagation", Status: model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, stack.DB.Create(&binding).Error)
	stub := &platformBindingRouteStub{}
	seedEnabledPlatformService(t, stack, startPlatformBindingRouteServer(t, stub))
	path := fmt.Sprintf("/api/v1/me/platform-accounts/%d/consumer-grants/paigram-bot", binding.ID)

	created := performJSONRequest(t, stack.Router, http.MethodPut, path, map[string]any{"enabled": true}, authHeaders(accessToken))
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	stub.authorizationFenceErr = grpcstatus.Error(codes.Unavailable, "fence response lost")

	revoked := performJSONRequest(t, stack.Router, http.MethodPut, path, map[string]any{"enabled": false}, authHeaders(accessToken))
	require.Equal(t, http.StatusAccepted, revoked.Code, revoked.Body.String())
	data := decodeResponseData(t, revoked)
	assert.Equal(t, "propagation_pending", data["state"])

	var grant model.ConsumerGrant
	require.NoError(t, stack.DB.Where("binding_id = ? AND consumer = ?", binding.ID, "paigram-bot").Take(&grant).Error)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, grant.Status)
	assert.False(t, grant.LastInvalidatedAt.Valid)

	stub.authorizationFenceErr = nil
	retried := performJSONRequest(t, stack.Router, http.MethodPut, path, map[string]any{"enabled": false}, authHeaders(accessToken))
	require.Equal(t, http.StatusOK, retried.Code, retried.Body.String())
	require.NoError(t, stack.DB.Where("binding_id = ? AND consumer = ?", binding.ID, "paigram-bot").Take(&grant).Error)
	assert.True(t, grant.LastInvalidatedAt.Valid)
}
