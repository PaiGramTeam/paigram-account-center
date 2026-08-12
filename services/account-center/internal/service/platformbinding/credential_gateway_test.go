package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"testing"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"paigram/internal/model"
)

func TestGRPCGenericCredentialGatewayBindCredentialUsesControlRPC(t *testing.T) {
	stub := &genericCredentialGatewayStub{bindResponse: &platformv2.BindCredentialResponse{Result: successfulCredentialOperationResult()}}
	gateway := newTestCredentialGateway(t, stub)
	binding := &model.PlatformAccountBinding{BindingRef: "binding-101"}

	summary, err := gateway.BindCredential(context.Background(), "bufnet", "ticket-123", "op-bind", binding, json.RawMessage(`{"cookie_bundle":"abc"}`))
	require.NoError(t, err)
	require.NotNil(t, stub.lastBind)
	require.Equal(t, []string{"Bearer ticket-123"}, stub.lastAuthorization)
	require.Equal(t, `{"cookie_bundle":"abc"}`, stub.lastBind.GetCredentialPayloadJson())
	requireOperation(t, stub.lastBind.GetOperation(), platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, "binding-101", 0, 1)
	require.Equal(t, "account-101", summary["platform_account_id"])
	require.Equal(t, uint64(1), summary["generation"])
}

func TestGRPCGenericCredentialGatewayReplaceCredentialUsesControlRPC(t *testing.T) {
	stub := &genericCredentialGatewayStub{replaceResponse: &platformv2.ReplaceCredentialResponse{Result: successfulCredentialOperationResult()}}
	gateway := newTestCredentialGateway(t, stub)
	binding := &model.PlatformAccountBinding{
		BindingRef:         "binding-101",
		Generation:         4,
		ExternalAccountKey: sql.NullString{String: "account-101", Valid: true},
	}

	_, err := gateway.ReplaceCredential(context.Background(), "bufnet", "ticket-123", "op-replace", binding, json.RawMessage(`{"cookie_bundle":"updated"}`))
	require.NoError(t, err)
	require.Equal(t, "account-101", stub.lastReplace.GetAccountKey())
	require.Equal(t, `{"cookie_bundle":"updated"}`, stub.lastReplace.GetCredentialPayloadJson())
	requireOperation(t, stub.lastReplace.GetOperation(), platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, "binding-101", 4, 5)
	require.Equal(t, []string{"Bearer ticket-123"}, stub.lastAuthorization)
}

func TestGRPCGenericCredentialGatewayDeleteCredentialUsesResolvedAccountKey(t *testing.T) {
	stub := &genericCredentialGatewayStub{deleteResponse: &platformv2.DeleteCredentialResponse{Result: successfulCredentialOperationResult()}}
	gateway := newTestCredentialGateway(t, stub)
	binding := &model.PlatformAccountBinding{
		BindingRef:         "binding-101",
		Generation:         4,
		ExternalAccountKey: sql.NullString{String: "account-101", Valid: true},
	}

	err := gateway.DeleteCredential(context.Background(), "bufnet", "ticket-123", "op-delete", binding)
	require.NoError(t, err)
	require.Equal(t, "account-101", stub.lastDelete.GetAccountKey())
	requireOperation(t, stub.lastDelete.GetOperation(), platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL, "binding-101", 4, 5)
	require.Equal(t, []string{"Bearer ticket-123"}, stub.lastAuthorization)
}

func TestCredentialOperationUsesCallerIntentAndNonSensitiveTupleFingerprint(t *testing.T) {
	binding := &model.PlatformAccountBinding{BindingRef: "binding-101", Generation: 4}

	first := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, "op-one")
	retry := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, "op-one")
	changed := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, "op-two")

	require.Equal(t, first.GetOperationId(), retry.GetOperationId())
	require.NotEqual(t, first.GetOperationId(), changed.GetOperationId())
	require.Equal(t, first.GetRequestFingerprint(), changed.GetRequestFingerprint())
}

func TestGenericOperationResultMapRejectsNonSucceededOperation(t *testing.T) {
	summary, err := genericOperationResultMap(&platformv2.OperationResult{State: platformv2.OperationState_OPERATION_STATE_FAILED})

	require.ErrorIs(t, err, errGenericCredentialOperationFailed)
	require.Nil(t, summary)
}

func TestGenericOperationResultMapRejectsMismatchedOperation(t *testing.T) {
	expected := &platformv2.OperationRef{OperationId: "expected"}
	result := successfulCredentialOperationResult()
	result.Operation = &platformv2.OperationRef{OperationId: "other"}

	summary, err := genericOperationResultMapForOperation(result, expected)

	require.ErrorIs(t, err, errGenericCredentialOperationMismatch)
	require.Nil(t, summary)
}

func successfulCredentialOperationResult() *platformv2.OperationResult {
	return &platformv2.OperationResult{
		State:            platformv2.OperationState_OPERATION_STATE_SUCCEEDED,
		AccountKey:       "account-101",
		CredentialStatus: platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
	}
}

func requireOperation(t *testing.T, operation *platformv2.OperationRef, kind platformv2.OperationKind, bindingRef string, preGeneration, targetGeneration uint64) {
	t.Helper()
	require.NotEmpty(t, operation.GetOperationId())
	require.NotEmpty(t, operation.GetRequestFingerprint())
	require.Equal(t, kind, operation.GetKind())
	require.Equal(t, bindingRef, operation.GetBindingRef())
	require.Equal(t, preGeneration, operation.GetPreGeneration())
	require.Equal(t, targetGeneration, operation.GetTargetGeneration())
}

func newTestCredentialGateway(t *testing.T, stub *genericCredentialGatewayStub) *GRPCGenericCredentialGateway {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv2.RegisterPlatformControlServiceServer(server, stub)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	return NewGRPCGenericCredentialGateway(func(ctx context.Context, _ string) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	})
}

type genericCredentialGatewayStub struct {
	platformv2.UnimplementedPlatformControlServiceServer
	bindResponse      *platformv2.BindCredentialResponse
	replaceResponse   *platformv2.ReplaceCredentialResponse
	deleteResponse    *platformv2.DeleteCredentialResponse
	lastBind          *platformv2.BindCredentialRequest
	lastReplace       *platformv2.ReplaceCredentialRequest
	lastDelete        *platformv2.DeleteCredentialRequest
	lastAuthorization []string
}

func (s *genericCredentialGatewayStub) BindCredential(ctx context.Context, req *platformv2.BindCredentialRequest) (*platformv2.BindCredentialResponse, error) {
	s.captureAuthorization(ctx)
	s.lastBind = req
	return &platformv2.BindCredentialResponse{Result: resultForRequestedOperation(s.bindResponse.GetResult(), req.GetOperation())}, nil
}

func (s *genericCredentialGatewayStub) ReplaceCredential(ctx context.Context, req *platformv2.ReplaceCredentialRequest) (*platformv2.ReplaceCredentialResponse, error) {
	s.captureAuthorization(ctx)
	s.lastReplace = req
	return &platformv2.ReplaceCredentialResponse{Result: resultForRequestedOperation(s.replaceResponse.GetResult(), req.GetOperation())}, nil
}

func (s *genericCredentialGatewayStub) DeleteCredential(ctx context.Context, req *platformv2.DeleteCredentialRequest) (*platformv2.DeleteCredentialResponse, error) {
	s.captureAuthorization(ctx)
	s.lastDelete = req
	return &platformv2.DeleteCredentialResponse{Result: resultForRequestedOperation(s.deleteResponse.GetResult(), req.GetOperation())}, nil
}

func resultForRequestedOperation(result *platformv2.OperationResult, operation *platformv2.OperationRef) *platformv2.OperationResult {
	if result == nil {
		return nil
	}
	copy := *result
	copy.Operation = operation
	return &copy
}

func (s *genericCredentialGatewayStub) captureAuthorization(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.lastAuthorization = append([]string(nil), md.Get("authorization")...)
}
