package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"paigram/internal/model"
)

type primaryProfilePlatformStub struct {
	profileRef  string
	operationID string
	scopes      []string
}

func (s *primaryProfilePlatformStub) GetEnabledPlatform(string) (*model.PlatformService, error) {
	return &model.PlatformService{Endpoint: "127.0.0.1:9000"}, nil
}

func (s *primaryProfilePlatformStub) IssueBindingScopedTicket(_, _ string, _ *model.PlatformAccountBinding, scopes []string) (string, time.Time, error) {
	s.scopes = append([]string(nil), scopes...)
	return "service-ticket", time.Time{}, nil
}

func (s *primaryProfilePlatformStub) IssueBindingScopedOperationTicket(_, _ string, _ *model.PlatformAccountBinding, operationID string, scopes []string) (string, time.Time, error) {
	s.operationID = operationID
	s.scopes = append([]string(nil), scopes...)
	return "service-ticket", time.Time{}, nil
}

func (s *primaryProfilePlatformStub) IssueProfileScopedOperationTicket(_, _ string, _ *model.PlatformAccountBinding, profileRef, operationID string, scopes []string) (string, time.Time, error) {
	s.profileRef = profileRef
	s.operationID = operationID
	s.scopes = append([]string(nil), scopes...)
	return "service-ticket", time.Time{}, nil
}

type primaryProfileGatewayStub struct {
	summary           *RuntimeSummary
	setErr            error
	resolution        *CredentialOperationResolution
	setCalled         bool
	setOperationID    string
	setProfileRef     string
	resolvedReference CredentialOperationReference
}

func (s *primaryProfileGatewayStub) BindCredential(context.Context, string, string, string, *model.PlatformAccountBinding, json.RawMessage) (map[string]any, error) {
	panic("unexpected call")
}

func (s *primaryProfileGatewayStub) ReplaceCredential(context.Context, string, string, string, *model.PlatformAccountBinding, json.RawMessage) (map[string]any, error) {
	panic("unexpected call")
}

func (s *primaryProfileGatewayStub) RefreshCredential(context.Context, string, string, string, *model.PlatformAccountBinding) (*RuntimeSummary, error) {
	panic("unexpected call")
}

func (s *primaryProfileGatewayStub) DeleteCredential(context.Context, string, string, string, *model.PlatformAccountBinding) error {
	panic("unexpected call")
}

func (s *primaryProfileGatewayStub) SetPrimaryProfile(_ context.Context, _, _ string, operationID string, _ *model.PlatformAccountBinding, profileRef string) (*RuntimeSummary, error) {
	s.setCalled = true
	s.setOperationID = operationID
	s.setProfileRef = profileRef
	return s.summary, s.setErr
}

func (s *primaryProfileGatewayStub) ResolveCredentialOperation(_ context.Context, _, _ string, reference CredentialOperationReference) (*CredentialOperationResolution, error) {
	s.resolvedReference = reference
	return s.resolution, nil
}

func (s *primaryProfileGatewayStub) GetCredentialBindingState(context.Context, string, string, string) (*CredentialBindingState, error) {
	return nil, nil
}

func primaryProfileBinding() *model.PlatformAccountBinding {
	return &model.PlatformAccountBinding{
		ID: 101, OwnerUserID: 7, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		BindingRef: "bind_test", Generation: 4, ProfileRevision: 7,
		ExternalAccountKey: sql.NullString{String: "binding_101_10001", Valid: true},
		Status:             model.PlatformAccountBindingStatusActive,
	}
}

func primaryProfileSummary() *RuntimeSummary {
	return &RuntimeSummary{
		PlatformAccountID: "binding_101_10001", Generation: 4, Status: "active",
		ProfileSnapshotComplete: true, ProfileRevision: 8, ProfileObservedRevision: 8,
		Profiles: []map[string]any{{"profile_ref": "profile-stable", "game_biz": "hk4e_cn", "region": "cn_gf01", "player_id": "1008611", "nickname": "Traveler", "is_default": true}},
	}
}

func TestSetPrimaryProfileForOwnerUpdatesProjectionOnlyAfterExecutionPlaneSuccess(t *testing.T) {
	binding := primaryProfileBinding()
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &primaryProfilePlatformStub{}
	profileSyncer := &fakeProfileSyncer{profile: &model.PlatformAccountProfile{ID: 404, BindingID: 101, ProfileRef: "profile-stable"}}
	gateway := &primaryProfileGatewayStub{summary: primaryProfileSummary()}
	service := NewOrchestrationService(reader, platformService, gateway, profileSyncer)

	updated, err := service.SetPrimaryProfileForOwner(context.Background(), 7, 101, 404, "session:99")
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.True(t, gateway.setCalled)
	assert.Equal(t, "profile-stable", gateway.setProfileRef)
	assert.Equal(t, gateway.setOperationID, platformService.operationID)
	assert.Equal(t, "profile-stable", platformService.profileRef)
	assert.Equal(t, []string{platformaction.MihomoProfileWrite}, platformService.scopes)
	assert.True(t, profileSyncer.called)
	assert.Equal(t, uint64(8), profileSyncer.input.Revision)
	assert.False(t, profileSyncer.setPrimaryCalled)
}

func TestSetPrimaryProfileForOwnerDoesNotUpdateProjectionWhenExecutionPlaneFails(t *testing.T) {
	binding := primaryProfileBinding()
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	profileSyncer := &fakeProfileSyncer{profile: &model.PlatformAccountProfile{ID: 404, BindingID: 101, ProfileRef: "profile-stable"}}
	gateway := &primaryProfileGatewayStub{setErr: grpcstatus.Error(codes.PermissionDenied, "profile rejected")}
	service := NewOrchestrationService(reader, &primaryProfilePlatformStub{}, gateway, profileSyncer)

	updated, err := service.SetPrimaryProfileForOwner(context.Background(), 7, 101, 404, "session:99")
	require.ErrorIs(t, err, ErrPrimaryProfileNotOwned)
	assert.Nil(t, updated)
	assert.False(t, profileSyncer.called)
}

func TestSetPrimaryProfileResponseLossReconcilesSameDurableOperation(t *testing.T) {
	binding := primaryProfileBinding()
	reader := &fakeRuntimeSummaryBindingReader{binding: binding}
	platformService := &primaryProfilePlatformStub{}
	profileSyncer := &fakeProfileSyncer{profile: &model.PlatformAccountProfile{ID: 404, BindingID: 101, ProfileRef: "profile-stable"}}
	gateway := &primaryProfileGatewayStub{
		setErr: grpcstatus.Error(codes.Unavailable, "response lost"),
		resolution: &CredentialOperationResolution{
			State:   CredentialRemoteOperationSucceeded,
			Summary: primaryProfileSummary(),
		},
	}
	store := NewOperationIntentService(openOperationIntentTestDB(t))
	service := NewOrchestrationService(reader, platformService, gateway, profileSyncer, store)

	updated, err := service.SetPrimaryProfileForOwner(context.Background(), 7, 101, 404, "session:99")
	var pending *CredentialOperationPendingError
	require.ErrorAs(t, err, &pending)
	assert.Nil(t, updated)
	intent, getErr := store.Get(context.Background(), pending.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateUncertain, intent.State)
	assert.Equal(t, "profile-stable", intent.ProfileRef)
	assert.Equal(t, uint64(7), intent.ProfileRevision)
	assert.Equal(t, intent.OperationID, gateway.setOperationID)

	require.NoError(t, service.ReconcileCredentialOperation(context.Background(), pending.OperationID))
	intent, getErr = store.Get(context.Background(), pending.OperationID)
	require.NoError(t, getErr)
	assert.Equal(t, model.PlatformOperationIntentStateSucceeded, intent.State)
	assert.Equal(t, pending.OperationID, gateway.resolvedReference.OperationID)
	assert.Equal(t, "OPERATION_KIND_SET_PRIMARY_PROFILE", gateway.resolvedReference.Kind)
	assert.True(t, profileSyncer.called)
	assert.Equal(t, uint64(8), profileSyncer.input.Revision)
}
