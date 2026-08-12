package platformbinding

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

type fakeCredentialOperationResolver struct {
	*fakeCredentialGateway
	resolution     *CredentialOperationResolution
	bindingState   *CredentialBindingState
	resolveErr     error
	stateErr       error
	resolvedRef    CredentialOperationReference
	refreshCalled  bool
	deleteCalled   bool
	deliveryErr    error
	refreshSummary *RuntimeSummary
	primarySummary *RuntimeSummary
	primaryCalled  bool
	primaryBinding *model.PlatformAccountBinding
}

func (f *fakeCredentialOperationResolver) ResolveCredentialOperation(_ context.Context, _, _ string, reference CredentialOperationReference) (*CredentialOperationResolution, error) {
	f.resolvedRef = reference
	return f.resolution, f.resolveErr
}

func (f *fakeCredentialOperationResolver) GetCredentialBindingState(context.Context, string, string, string) (*CredentialBindingState, error) {
	return f.bindingState, f.stateErr
}

func (f *fakeCredentialOperationResolver) RefreshCredential(context.Context, string, string, string, *model.PlatformAccountBinding) (*RuntimeSummary, error) {
	f.refreshCalled = true
	return f.refreshSummary, f.deliveryErr
}

func (f *fakeCredentialOperationResolver) DeleteCredential(context.Context, string, string, string, *model.PlatformAccountBinding) error {
	f.deleteCalled = true
	return f.deliveryErr
}

func (f *fakeCredentialOperationResolver) SetPrimaryProfile(_ context.Context, _, _ string, _ string, binding *model.PlatformAccountBinding, _ string) (*RuntimeSummary, error) {
	f.primaryCalled = true
	f.primaryBinding = binding
	return f.primarySummary, f.deliveryErr
}

func newCredentialReconcileService(t *testing.T, resolver *fakeCredentialOperationResolver) (*OrchestrationService, *OperationIntentService, *fakeRuntimeSummaryBindingReader) {
	t.Helper()
	db := openOperationIntentTestDB(t)
	store := NewOperationIntentService(db)
	_, err := store.Admit(context.Background(), operationIntentInput("op_reconcile"))
	require.NoError(t, err)
	require.NoError(t, store.MarkUncertain(context.Background(), "op_reconcile", "delivery_outcome_unknown"))
	binding := &model.PlatformAccountBinding{
		ID: 101, BindingRef: "bind_test", OwnerUserID: 7, Platform: "mihomo",
		PlatformServiceKey: "platform-mihomo-service", Status: model.PlatformAccountBindingStatusPendingBind,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &fakeOrchestrationPlatformService{
		platform: &model.PlatformService{Endpoint: "127.0.0.1:9000"}, ticket: "service-ticket",
	}
	return NewOrchestrationService(reader, platformService, resolver, store), store, reader
}

func TestReconcileNotReceivedAndAbsentBindingRequiresNewInput(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution:            &CredentialOperationResolution{State: CredentialRemoteOperationNotReceived},
		bindingState:          &CredentialBindingState{Exists: false},
	}
	service, store, reader := newCredentialReconcileService(t, resolver)

	err := service.ReconcileCredentialOperation(context.Background(), "op_reconcile")
	require.NoError(t, err)
	intent, getErr := store.Get(context.Background(), "op_reconcile")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateInputRequired, intent.State)
	assert.Equal(t, "credential_input_required", reader.updatedReason)
	assert.Equal(t, "op_reconcile", resolver.resolvedRef.OperationID)

	_, err = store.Admit(context.Background(), CredentialOperationIntentInput{
		OperationID: "op_resubmitted", BindingID: 101, BindingRef: "bind_test", Kind: "OPERATION_KIND_BIND_CREDENTIAL",
		PreGeneration: 0, TargetGeneration: 1, RequestFingerprint: "new-fingerprint", ActorType: "user", ActorID: "session:new",
	})
	require.NoError(t, err)
}

func TestReconcileKeepsIntentOpenWhenInputRequiredProjectionFails(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution:            &CredentialOperationResolution{State: CredentialRemoteOperationNotReceived},
		bindingState:          &CredentialBindingState{Exists: false},
	}
	service, store, reader := newCredentialReconcileService(t, resolver)
	reader.updateErr = errors.New("database unavailable")

	err := service.ReconcileCredentialOperation(context.Background(), "op_reconcile")
	require.Error(t, err)
	intent, getErr := store.Get(context.Background(), "op_reconcile")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, intent.State)
}

func TestReconcileSucceededOperationRepairsProjectionAndCompletesIntent(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution: &CredentialOperationResolution{State: CredentialRemoteOperationSucceeded, Summary: &RuntimeSummary{
			PlatformAccountID: "account-101", Generation: 1, Status: "active",
			ProfileSnapshotComplete: true, ProfileRevision: 1, ProfileObservedRevision: 1,
		}},
	}
	service, store, reader := newCredentialReconcileService(t, resolver)

	err := service.ReconcileCredentialOperation(context.Background(), "op_reconcile")
	require.NoError(t, err)
	intent, getErr := store.Get(context.Background(), "op_reconcile")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
	require.NotNil(t, reader.persistedSummary)
	assert.Equal(t, uint64(1), reader.persistedSummary.Generation)
}

func TestReconcileMissingTerminalAtTargetGenerationKeepsInvariantReservation(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution:            &CredentialOperationResolution{State: CredentialRemoteOperationNotReceived},
		bindingState: &CredentialBindingState{Exists: true, Summary: &RuntimeSummary{
			PlatformAccountID: "account-101", Generation: 1,
		}},
	}
	service, store, reader := newCredentialReconcileService(t, resolver)

	err := service.ReconcileCredentialOperation(context.Background(), "op_reconcile")
	require.ErrorIs(t, err, ErrBindingGenerationConflict)
	intent, getErr := store.Get(context.Background(), "op_reconcile")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateInvariantViolation, intent.State)
	assert.True(t, intent.State.ReservesBinding())
	assert.Nil(t, reader.persistedSummary)
}

func TestReconcileUnexpectedBindingGenerationKeepsInvariantReservation(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution:            &CredentialOperationResolution{State: CredentialRemoteOperationNotReceived},
		bindingState: &CredentialBindingState{Exists: true, Summary: &RuntimeSummary{
			PlatformAccountID: "account-101", Generation: 2,
		}},
	}
	service, store, reader := newCredentialReconcileService(t, resolver)

	err := service.ReconcileCredentialOperation(context.Background(), "op_reconcile")
	require.ErrorIs(t, err, ErrBindingGenerationConflict)
	intent, getErr := store.Get(context.Background(), "op_reconcile")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateInvariantViolation, intent.State)
	assert.True(t, intent.State.ReservesBinding())
	assert.Equal(t, "operation_invariant_violation", reader.updatedReason)
}

func newNonSensitiveReconcileService(t *testing.T, resolver *fakeCredentialOperationResolver, uncertain bool) (*OrchestrationService, *OperationIntentService, *fakeRuntimeSummaryBindingReader) {
	t.Helper()
	store := NewOperationIntentService(openOperationIntentTestDB(t))
	input := CredentialOperationIntentInput{
		OperationID: "op_refresh", BindingID: 101, BindingRef: "bind_test", Kind: "OPERATION_KIND_REFRESH_CREDENTIAL",
		PreGeneration: 4, TargetGeneration: 5, RequestFingerprint: "refresh-fingerprint", ActorType: "user", ActorID: "session:test",
	}
	_, err := store.Admit(context.Background(), input)
	require.NoError(t, err)
	if uncertain {
		require.NoError(t, store.MarkUncertain(context.Background(), input.OperationID, "delivery_outcome_unknown"))
	}
	binding := &model.PlatformAccountBinding{
		ID: 101, BindingRef: "bind_test", OwnerUserID: 7, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		Generation: 4, ExternalAccountKey: sql.NullString{String: "account-101", Valid: true}, Status: model.PlatformAccountBindingStatusActive,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &fakeOrchestrationPlatformService{platform: &model.PlatformService{Endpoint: "127.0.0.1:9000"}, ticket: "service-ticket"}
	return NewOrchestrationService(reader, platformService, resolver, store), store, reader
}

func TestReconcileDeliversPendingNonSensitiveRefresh(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		refreshSummary: &RuntimeSummary{
			PlatformAccountID: "account-101", Generation: 5, Status: "active",
			ProfileSnapshotComplete: true, ProfileRevision: 2, ProfileObservedRevision: 2,
		},
	}
	service, store, reader := newNonSensitiveReconcileService(t, resolver, false)

	err := service.ReconcileCredentialOperation(context.Background(), "op_refresh")
	require.NoError(t, err)
	assert.True(t, resolver.refreshCalled)
	assert.Equal(t, model.PlatformAccountBindingStatusActive, reader.binding.Status)
	intent, getErr := store.Get(context.Background(), "op_refresh")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
}

func TestReconcilePrimaryReplayUsesIntentProfileRevision(t *testing.T) {
	store := NewOperationIntentService(openOperationIntentTestDB(t))
	input := CredentialOperationIntentInput{
		OperationID: "op_primary", BindingID: 101, BindingRef: "bind_test", Kind: "OPERATION_KIND_SET_PRIMARY_PROFILE",
		PreGeneration: 4, TargetGeneration: 4, RequestFingerprint: "primary-fingerprint",
		ProfileRef: "profile-stable", ProfileRevision: 7, ActorType: "user", ActorID: "session:test",
	}
	_, err := store.Admit(context.Background(), input)
	require.NoError(t, err)
	binding := &model.PlatformAccountBinding{
		ID: 101, BindingRef: "bind_test", OwnerUserID: 7, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		Generation: 4, ProfileRevision: 8, ExternalAccountKey: sql.NullString{String: "account-101", Valid: true}, Status: model.PlatformAccountBindingStatusActive,
	}
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		primarySummary: &RuntimeSummary{
			PlatformAccountID: "account-101", Generation: 4, Status: "active",
			ProfileSnapshotComplete: true, ProfileRevision: 8, ProfileObservedRevision: 8,
		},
	}
	platformService := &fakeOrchestrationPlatformService{platform: &model.PlatformService{Endpoint: "127.0.0.1:9000"}, ticket: "service-ticket"}
	service := NewOrchestrationService(reader, platformService, resolver, store)

	require.NoError(t, service.ReconcileCredentialOperation(context.Background(), input.OperationID))
	require.True(t, resolver.primaryCalled)
	require.NotNil(t, resolver.primaryBinding)
	assert.Equal(t, uint64(7), resolver.primaryBinding.ProfileRevision)
	intent, getErr := store.Get(context.Background(), input.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
}

func TestReconcileRetriesNonSensitiveOperationOnlyAfterNotReceivedAndPreGenerationProof(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution:            &CredentialOperationResolution{State: CredentialRemoteOperationNotReceived},
		bindingState: &CredentialBindingState{Exists: true, Summary: &RuntimeSummary{
			PlatformAccountID: "account-101", Generation: 4,
		}},
	}
	service, store, _ := newNonSensitiveReconcileService(t, resolver, true)

	err := service.ReconcileCredentialOperation(context.Background(), "op_refresh")
	require.NoError(t, err)
	oldIntent, getErr := store.Get(context.Background(), "op_refresh")
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateSuperseded, oldIntent.State)

	operationIDs, listErr := store.ClaimDueOperationIDs(context.Background(), time.Now().UTC().Add(credentialOperationInitialDeliveryLease), 10)
	require.NoError(t, listErr)
	require.Len(t, operationIDs, 1)
	assert.NotEqual(t, "op_refresh", operationIDs[0])
	retryIntent, getErr := store.Get(context.Background(), operationIDs[0])
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStatePendingDelivery, retryIntent.State)
}

func TestReconcileDeleteTreatsAbsentBindingAsIdempotentSuccess(t *testing.T) {
	resolver := &fakeCredentialOperationResolver{
		fakeCredentialGateway: &fakeCredentialGateway{},
		resolution:            &CredentialOperationResolution{State: CredentialRemoteOperationNotReceived},
		bindingState:          &CredentialBindingState{Exists: false},
	}
	service, store, reader := newNonSensitiveReconcileService(t, resolver, true)
	var intent model.PlatformOperationIntent
	require.NoError(t, store.db.Where("operation_id = ?", "op_refresh").Take(&intent).Error)
	require.NoError(t, store.db.Model(&intent).Update("kind", "OPERATION_KIND_DELETE_CREDENTIAL").Error)

	require.NoError(t, service.ReconcileCredentialOperation(context.Background(), "op_refresh"))
	stored, err := store.Get(context.Background(), "op_refresh")
	require.NoError(t, err)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, stored.State)
	assert.Equal(t, uint64(101), reader.deletedID)
}
