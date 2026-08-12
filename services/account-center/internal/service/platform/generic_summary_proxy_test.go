package platform

import (
	"context"
	"errors"
	"net"
	"testing"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

func TestGRPCGenericSummaryProxyGetCredentialSummary(t *testing.T) {
	server := &genericPlatformServiceStub{
		response: &platformv2.GetBindingStateResponse{State: &platformv2.BindingState{
			Exists:               true,
			BindingRef:           "binding-11",
			AccountKey:           "account-11",
			CredentialGeneration: 4,
			CredentialStatus:     platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE,
			ProfileSnapshot: &platformv2.ProfileSnapshot{Complete: true, Revision: 4, ObservedRevision: 4, Profiles: []*platformv2.ProfileSummary{{
				ProfileRef: "profile-42",
				AccountKey: "account-11",
				GameBiz:    "hk4e_global",
				Region:     "os_usa",
				PlayerId:   "10001",
				Nickname:   "Traveler",
				Level:      60,
				IsDefault:  true,
			}}},
		}},
	}

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	platformv2.RegisterPlatformControlServiceServer(grpcServer, server)

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
		<-serveErrCh
	})

	proxy := NewGRPCGenericSummaryProxy(func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		require.Equal(t, "bufnet", endpoint)
		return grpc.DialContext(ctx, "passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	})

	summary, err := proxy.GetCredentialSummary(context.Background(), "bufnet", "ticket-123", "binding-11", "account-11")
	require.NoError(t, err)
	require.Equal(t, []string{"Bearer ticket-123"}, server.lastAuthorization)
	require.Equal(t, "binding-11", server.lastRequest.GetBindingRef())
	require.Equal(t, map[string]any{
		"platform_account_id": "account-11",
		"generation":          uint64(4),
		"status":              "active",
		"last_validated_at":   nil,
		"last_refreshed_at":   nil,
		"devices":             []map[string]any{},
		"profiles": []map[string]any{{
			"profile_ref":         "profile-42",
			"platform_account_id": "account-11",
			"game_biz":            "hk4e_global",
			"region":              "os_usa",
			"player_id":           "10001",
			"nickname":            "Traveler",
			"level":               int32(60),
			"is_default":          true,
		}},
		"profile_snapshot_complete": true,
		"profile_revision":          uint64(4),
		"profile_observed_revision": uint64(4),
	}, summary)
}

func TestGRPCGenericSummaryProxyRejectsMismatchedAccountKey(t *testing.T) {
	server := &genericPlatformServiceStub{response: &platformv2.GetBindingStateResponse{State: &platformv2.BindingState{
		Exists: true, BindingRef: "binding-11", AccountKey: "other-account",
	}}}
	proxy := newTestGenericSummaryProxy(t, server)

	_, err := proxy.GetCredentialSummary(context.Background(), "bufnet", "ticket-123", "binding-11", "account-11")
	require.ErrorIs(t, err, ErrPlatformServiceUnavailable)
}

func TestGRPCGenericSummaryProxyPropagatesRPCError(t *testing.T) {
	proxy := NewGRPCGenericSummaryProxy(func(context.Context, string) (*grpc.ClientConn, error) {
		return nil, errors.New("dial failed")
	})

	_, err := proxy.GetCredentialSummary(context.Background(), "bufnet", "ticket-123", "binding-11", "account-11")
	require.Error(t, err)
}

func newTestGenericSummaryProxy(t *testing.T, server *genericPlatformServiceStub) *GRPCGenericSummaryProxy {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	platformv2.RegisterPlatformControlServiceServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	return NewGRPCGenericSummaryProxy(func(ctx context.Context, _ string) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	})
}

type genericPlatformServiceStub struct {
	platformv2.UnimplementedPlatformControlServiceServer
	response          *platformv2.GetBindingStateResponse
	lastRequest       *platformv2.GetBindingStateRequest
	lastAuthorization []string
}

func (s *genericPlatformServiceStub) GetBindingState(ctx context.Context, req *platformv2.GetBindingStateRequest) (*platformv2.GetBindingStateResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.lastAuthorization = append([]string(nil), md.Get("authorization")...)
	s.lastRequest = req
	return s.response, nil
}
