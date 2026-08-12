package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func TestPutCredentialForOwnerReplacesResolvedCredential(t *testing.T) {
	binding := &model.PlatformAccountBinding{
		ID:                 101,
		OwnerUserID:        7,
		Platform:           "mihomo",
		ExternalAccountKey: sql.NullString{String: "cn:resolved-account", Valid: true},
		Status:             model.PlatformAccountBindingStatusActive,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformSvc := &fakeOrchestrationPlatformService{
		platform: &model.PlatformService{Endpoint: "127.0.0.1:9000"},
		ticket:   "service-ticket",
	}
	gateway := &fakeCredentialGateway{summary: map[string]any{
		"platform_account_id": "cn:resolved-account",
		"status":              "active",
	}}
	svc := NewOrchestrationService(reader, platformSvc, gateway)

	summary, err := svc.PutCredentialForOwner(context.Background(), PutCredentialInput{
		OwnerUserID:       7,
		BindingID:         101,
		ActorType:         "user",
		ActorID:           "session:99",
		CredentialPayload: json.RawMessage(`{"cookie_bundle":"replacement"}`),
	})
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, "replace", gateway.lastMutation)
	assert.Equal(t, []string{"mihomo.credential.update"}, platformSvc.lastScope)
}
