package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"paigram/internal/model"
)

func newCoordinatedCredentialService(t *testing.T, gateway *fakeCredentialGateway) (*OrchestrationService, *OperationIntentService, *fakeRuntimeSummaryBindingReader) {
	t.Helper()
	db := openOperationIntentTestDB(t)
	store := NewOperationIntentService(db)
	binding := &model.PlatformAccountBinding{
		ID: 101, BindingRef: "bind_test", OwnerUserID: 7, Platform: "mihomo",
		PlatformServiceKey: "platform-mihomo-service", Status: model.PlatformAccountBindingStatusPendingBind,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &fakeOrchestrationPlatformService{
		platform: &model.PlatformService{Endpoint: "127.0.0.1:9000"}, ticket: "service-ticket",
	}
	return NewOrchestrationService(reader, platformService, gateway, store), store, reader
}

func coordinatedPutInput() PutCredentialInput {
	return PutCredentialInput{
		OwnerUserID: 7, BindingID: 101, ActorType: "user", ActorID: "session:test",
		CredentialPayload: json.RawMessage(`{"cookie_bundle":"secret-cookie","device_id":"device","device_fp":"fingerprint"}`),
	}
}

func TestCredentialDeliveryOutageEntersUncertainAndKeepsOutboxPending(t *testing.T) {
	service, store, reader := newCoordinatedCredentialService(t, &fakeCredentialGateway{
		err: grpcstatus.Error(codes.Unavailable, "response lost"),
	})

	summary, err := service.PutCredentialForOwner(context.Background(), coordinatedPutInput())
	var pending *CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	assert.Nil(t, summary)
	assert.Equal(t, uint64(101), pending.BindingID)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, pending.State)
	assert.False(t, reader.updated)

	intent, getErr := store.Get(context.Background(), pending.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, intent.State)
	var outbox model.PlatformOperationOutbox
	require.NoError(t, store.db.Where("operation_id = ?", pending.OperationID).Take(&outbox).Error)
	assert.Equal(t, model.PlatformOperationOutboxStatusPending, outbox.Status)
}

func TestCredentialDeliveryValidationFailureReleasesReservation(t *testing.T) {
	service, store, _ := newCoordinatedCredentialService(t, &fakeCredentialGateway{
		err: grpcstatus.Error(codes.InvalidArgument, "invalid credential"),
	})

	_, err := service.PutCredentialForOwner(context.Background(), coordinatedPutInput())
	require.ErrorIs(t, err, ErrCredentialValidationFailed)

	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Take(&intent).Error)
	assert.Equal(t, model.PlatformOperationIntentStateFailed, intent.State)
	var outbox model.PlatformOperationOutbox
	require.NoError(t, store.db.Where("operation_id = ?", intent.OperationID).Take(&outbox).Error)
	assert.Equal(t, model.PlatformOperationOutboxStatusCompleted, outbox.Status)
}

func TestCredentialBindOwnershipConflictDeletesDraftAndReturnsTypedConflict(t *testing.T) {
	service, store, reader := newCoordinatedCredentialService(t, &fakeCredentialGateway{
		err: grpcstatus.Error(codes.AlreadyExists, "account already bound"),
	})

	_, err := service.PutCredentialForOwner(context.Background(), coordinatedPutInput())
	require.ErrorIs(t, err, ErrBindingAlreadyOwned)
	assert.Equal(t, uint64(101), reader.deletedID)
	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Take(&intent).Error)
	assert.Equal(t, model.PlatformOperationIntentStateFailed, intent.State)
}

func TestCredentialDeliverySuccessCompletesIntentAfterProjection(t *testing.T) {
	gateway := &fakeCredentialGateway{summary: map[string]any{
		"platform_account_id":       "account-101",
		"generation":                uint64(1),
		"status":                    "active",
		"profiles":                  []map[string]any{},
		"profile_snapshot_complete": true,
		"profile_revision":          uint64(1),
		"profile_observed_revision": uint64(1),
	}}
	service, store, reader := newCoordinatedCredentialService(t, gateway)

	summary, err := service.PutCredentialForOwner(context.Background(), coordinatedPutInput())
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, "account-101", summary.PlatformAccountID)
	assert.Equal(t, sql.NullString{String: "account-101", Valid: true}, reader.binding.ExternalAccountKey)

	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Take(&intent).Error)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
}

type fakeNonSensitiveCredentialGateway struct {
	refreshErr    error
	deleteErr     error
	refreshCalled bool
	deleteCalled  bool
}

func (f *fakeNonSensitiveCredentialGateway) BindCredential(context.Context, string, string, string, *model.PlatformAccountBinding, json.RawMessage) (map[string]any, error) {
	panic("unexpected bind")
}

func (f *fakeNonSensitiveCredentialGateway) ReplaceCredential(context.Context, string, string, string, *model.PlatformAccountBinding, json.RawMessage) (map[string]any, error) {
	panic("unexpected replace")
}

func (f *fakeNonSensitiveCredentialGateway) RefreshCredential(context.Context, string, string, string, *model.PlatformAccountBinding) error {
	f.refreshCalled = true
	return f.refreshErr
}

func (f *fakeNonSensitiveCredentialGateway) DeleteCredential(context.Context, string, string, string, *model.PlatformAccountBinding) error {
	f.deleteCalled = true
	return f.deleteErr
}

func (f *fakeNonSensitiveCredentialGateway) SetPrimaryProfile(context.Context, string, string, string, *model.PlatformAccountBinding, string) (*RuntimeSummary, error) {
	panic("unexpected call")
}

func newDurableNonSensitiveService(t *testing.T, gateway credentialGateway) (*OrchestrationService, *OperationIntentService, *fakeRuntimeSummaryBindingReader) {
	t.Helper()
	store := NewOperationIntentService(openOperationIntentTestDB(t))
	binding := &model.PlatformAccountBinding{
		ID: 101, BindingRef: "bind_test", OwnerUserID: 7, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		Generation: 4, ExternalAccountKey: sql.NullString{String: "account-101", Valid: true}, Status: model.PlatformAccountBindingStatusActive,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &fakeOrchestrationPlatformService{platform: &model.PlatformService{Endpoint: "127.0.0.1:9000"}, ticket: "service-ticket"}
	return NewOrchestrationService(reader, platformService, gateway, store), store, reader
}

func TestRefreshDeliveryOutageUsesDurableUncertainIntent(t *testing.T) {
	gateway := &fakeNonSensitiveCredentialGateway{refreshErr: grpcstatus.Error(codes.Unavailable, "response lost")}
	service, store, _ := newDurableNonSensitiveService(t, gateway)

	_, err := service.RefreshBindingForOwner(context.Background(), 7, 101)
	var pending *CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	assert.True(t, gateway.refreshCalled)
	intent, getErr := store.Get(context.Background(), pending.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, "OPERATION_KIND_REFRESH_CREDENTIAL", intent.Kind)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, intent.State)
}

func TestDeleteDeliveryOutageDoesNotReportDeleteFailedOrReleaseIntent(t *testing.T) {
	gateway := &fakeNonSensitiveCredentialGateway{deleteErr: grpcstatus.Error(codes.DeadlineExceeded, "response lost")}
	service, store, reader := newDurableNonSensitiveService(t, gateway)

	err := service.DeleteBindingForOwner(context.Background(), 7, 101)
	var pending *CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	assert.True(t, gateway.deleteCalled)
	assert.Equal(t, model.PlatformAccountBindingStatusDeleting, reader.binding.Status)
	assert.Empty(t, reader.updatedReason)
	intent, getErr := store.Get(context.Background(), pending.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, "OPERATION_KIND_DELETE_CREDENTIAL", intent.Kind)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, intent.State)
}

func TestDeleteDefinitiveFailurePersistsDeleteFailedBeforeClosingIntent(t *testing.T) {
	gateway := &fakeNonSensitiveCredentialGateway{deleteErr: grpcstatus.Error(codes.PermissionDenied, "delete rejected")}
	service, store, reader := newDurableNonSensitiveService(t, gateway)

	err := service.DeleteBindingForOwner(context.Background(), 7, 101)
	require.Error(t, err)
	assert.Equal(t, model.PlatformAccountBindingStatusDeleteFailed, reader.binding.Status)
	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Take(&intent).Error)
	assert.Equal(t, model.PlatformOperationIntentStateFailed, intent.State)
}

func TestCredentialResponseLossConvergesThroughResolveRPC(t *testing.T) {
	stub := &genericCredentialGatewayStub{
		loseBindResponseAfterCommit: true,
		bindResponse: &platformv2.BindCredentialResponse{Result: &platformv2.OperationResult{
			State: platformv2.OperationState_OPERATION_STATE_SUCCEEDED, AccountKey: "account-101",
			CredentialStatus: platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
			ProfileSnapshot:  &platformv2.ProfileSnapshot{Complete: true, Revision: 1, ObservedRevision: 1},
		}},
	}
	gateway := newTestCredentialGateway(t, stub)
	service, store, reader := newCoordinatedCredentialService(t, nil)
	service.gateway = gateway

	_, err := service.PutCredentialForOwner(context.Background(), coordinatedPutInput())
	var pending *CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	require.NoError(t, service.ReconcileCredentialOperation(context.Background(), pending.OperationID))

	intent, getErr := store.Get(context.Background(), pending.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
	require.NotNil(t, reader.persistedSummary)
	assert.Equal(t, "account-101", reader.persistedSummary.PlatformAccountID)
	assert.Equal(t, uint64(1), reader.persistedSummary.Generation)
}
