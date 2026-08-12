package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"testing"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"paigram/internal/model"
)

func TestGRPCGenericCredentialGatewayBindCredentialUsesDedicatedRPC(t *testing.T) {
	stub := &genericCredentialGatewayStub{bindResponse: &platformv1.BindCredentialResponse{Summary: &platformv1.GetCredentialSummaryResponse{
		PlatformAccountId: "binding_101_10001",
		Status:            platformv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
		LastValidatedAt:   timestamppb.Now(),
	}}}
	gateway := newTestCredentialGateway(t, stub)

	summary, err := gateway.BindCredential(context.Background(), "bufnet", "ticket-123", &model.PlatformAccountBinding{}, json.RawMessage(`{"cookie_bundle":"abc"}`))
	require.NoError(t, err)
	require.NotNil(t, stub.lastBind)
	require.Equal(t, []string{"Bearer ticket-123"}, stub.lastAuthorization)
	require.Equal(t, `{"cookie_bundle":"abc"}`, stub.lastBind.GetCredentialPayloadJson())
	require.Equal(t, "binding_101_10001", summary["platform_account_id"])
}

func TestGRPCGenericCredentialGatewayReplaceCredentialUsesDedicatedRPC(t *testing.T) {
	stub := &genericCredentialGatewayStub{replaceResponse: &platformv1.ReplaceCredentialResponse{Summary: &platformv1.GetCredentialSummaryResponse{
		PlatformAccountId: "binding_101_10001",
	}}}
	gateway := newTestCredentialGateway(t, stub)
	binding := &model.PlatformAccountBinding{ExternalAccountKey: sql.NullString{String: "binding_101_10001", Valid: true}}

	_, err := gateway.ReplaceCredential(context.Background(), "bufnet", "ticket-123", binding, json.RawMessage(`{"cookie_bundle":"updated"}`))
	require.NoError(t, err)
	require.Equal(t, "binding_101_10001", stub.lastReplace.GetPlatformAccountId())
	require.Equal(t, `{"cookie_bundle":"updated"}`, stub.lastReplace.GetCredentialPayloadJson())
	require.Equal(t, []string{"Bearer ticket-123"}, stub.lastAuthorization)
}

func TestGRPCGenericCredentialGatewayDeleteCredentialUsesResolvedAccountKey(t *testing.T) {
	stub := &genericCredentialGatewayStub{deleteResponse: &platformv1.DeleteCredentialResponse{Success: true}}
	gateway := newTestCredentialGateway(t, stub)
	binding := &model.PlatformAccountBinding{ExternalAccountKey: sql.NullString{String: "binding_101_10001", Valid: true}}

	err := gateway.DeleteCredential(context.Background(), "bufnet", "ticket-123", binding)
	require.NoError(t, err)
	require.Equal(t, "binding_101_10001", stub.lastDelete.GetPlatformAccountId())
	require.Equal(t, []string{"Bearer ticket-123"}, stub.lastAuthorization)
}

func newTestCredentialGateway(t *testing.T, stub *genericCredentialGatewayStub) *GRPCGenericCredentialGateway {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	platformv1.RegisterPlatformServiceServer(server, stub)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	return NewGRPCGenericCredentialGateway(func(ctx context.Context, _ string) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	})
}

type genericCredentialGatewayStub struct {
	platformv1.UnimplementedPlatformServiceServer
	bindResponse      *platformv1.BindCredentialResponse
	replaceResponse   *platformv1.ReplaceCredentialResponse
	deleteResponse    *platformv1.DeleteCredentialResponse
	lastBind          *platformv1.BindCredentialRequest
	lastReplace       *platformv1.ReplaceCredentialRequest
	lastDelete        *platformv1.DeleteCredentialRequest
	lastAuthorization []string
}

func (s *genericCredentialGatewayStub) BindCredential(ctx context.Context, req *platformv1.BindCredentialRequest) (*platformv1.BindCredentialResponse, error) {
	s.captureAuthorization(ctx)
	s.lastBind = req
	return s.bindResponse, nil
}

func (s *genericCredentialGatewayStub) ReplaceCredential(ctx context.Context, req *platformv1.ReplaceCredentialRequest) (*platformv1.ReplaceCredentialResponse, error) {
	s.captureAuthorization(ctx)
	s.lastReplace = req
	return s.replaceResponse, nil
}

func (s *genericCredentialGatewayStub) DeleteCredential(ctx context.Context, req *platformv1.DeleteCredentialRequest) (*platformv1.DeleteCredentialResponse, error) {
	s.captureAuthorization(ctx)
	s.lastDelete = req
	return s.deleteResponse, nil
}

func (s *genericCredentialGatewayStub) captureAuthorization(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.lastAuthorization = append([]string(nil), md.Get("authorization")...)
}
