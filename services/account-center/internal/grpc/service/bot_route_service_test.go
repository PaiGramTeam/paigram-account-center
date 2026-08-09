package service

import (
	"context"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"paigram/internal/grpc/interceptor"
	pb "paigram/internal/grpc/pb/v1"
	"paigram/internal/model"
	"paigram/internal/service/botroute"
	"paigram/internal/service/credentials"
	"paigram/internal/testutil"
)

// contextWithScopes returns a context populated with a credentials.AccessClaims
// carrying the given scopes — mirrors what the AuthInterceptor injects after
// validating an HS256 OAuth access token (Path D §1.1). Tests use this to
// exercise the scope-based authorization branches without spinning up the
// full interceptor pipeline.
func contextWithScopes(clientID string, scopes ...string) context.Context {
	claims := &credentials.AccessClaims{
		ClientID: clientID,
		Scope:    strings.Join(scopes, " "),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  clientID,
			Issuer:   "account-center",
			Audience: jwt.ClaimStrings{"account-center"},
		},
	}
	return interceptor.WithCredentialClaims(context.Background(), claims)
}

func newGRPCBotRouteService(t *testing.T, prefix string) *BotRouteService {
	t.Helper()
	db := testutil.OpenMySQLTestDB(t, prefix, &model.BotRoute{}, &model.BotRouteAudit{})
	return NewBotRouteService(botroute.NewService(db, nil))
}

func TestBotRouteService_RegisterRequiresAuthenticatedCaller(t *testing.T) {
	svc := NewBotRouteService(botroute.NewService(nil, nil))

	_, err := svc.RegisterBotService(context.Background(), &pb.RegisterBotServiceRequest{BotId: "paigrambot"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "expected gRPC status error")
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestBotRouteService_RegisterRejectsCallerWithoutManageScope(t *testing.T) {
	svc := NewBotRouteService(botroute.NewService(nil, nil))
	// Caller has only the read scope; should fail the manage check.
	ctx := contextWithScopes("paigrambot", "bot.route.read")

	_, err := svc.RegisterBotService(ctx, &pb.RegisterBotServiceRequest{
		BotId:    "paigrambot",
		Platform: "telegram",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestBotRouteService_RegisterSucceedsWithManageScope(t *testing.T) {
	svc := newGRPCBotRouteService(t, "grpc_botroute_register_ok")
	ctx := contextWithScopes("paigrambot", "bot.route.manage")

	resp, err := svc.RegisterBotService(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
		Endpoint:  "paigram-genshin:50052",
		Version:   "1.0.0",
	})
	require.NoError(t, err)
	assert.NotZero(t, resp.GetRegisteredAtUnix())
}

func TestBotRouteService_UnregisterRejectsCallerWithoutManageScope(t *testing.T) {
	svc := NewBotRouteService(botroute.NewService(nil, nil))
	ctx := contextWithScopes("paigrambot", "bot.route.read")

	_, err := svc.UnregisterBotService(ctx, &pb.UnregisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestBotRouteService_UnregisterReturnsFailedPreconditionOnServiceIDMismatch(t *testing.T) {
	svc := newGRPCBotRouteService(t, "grpc_botroute_unreg_mismatch")
	ctx := contextWithScopes("paigrambot", "bot.route.manage")

	_, err := svc.RegisterBotService(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin-v2",
		Endpoint:  "paigram-genshin-blue:50052",
		Version:   "2.0.0",
	})
	require.NoError(t, err)

	_, err = svc.UnregisterBotService(ctx, &pb.UnregisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin-v1",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

func TestBotRouteService_GetBotRouteReturnsNotFound(t *testing.T) {
	svc := newGRPCBotRouteService(t, "grpc_botroute_get_missing")
	ctx := contextWithScopes("paigrambot", "bot.route.read")

	_, err := svc.GetBotRoute(ctx, &pb.GetBotRouteRequest{
		BotId:    "paigrambot",
		Platform: "telegram",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestBotRouteService_GetBotRouteRequiresReadScope(t *testing.T) {
	svc := NewBotRouteService(botroute.NewService(nil, nil))
	// No bot.route.read or bot.route.manage — should be denied even though
	// the credential is authenticated.
	ctx := contextWithScopes("paigrambot", "unrelated.scope")

	_, err := svc.GetBotRoute(ctx, &pb.GetBotRouteRequest{
		BotId:    "paigrambot",
		Platform: "telegram",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestBotRouteService_GetBotRouteAcceptsManageScopeForRead(t *testing.T) {
	svc := newGRPCBotRouteService(t, "grpc_botroute_get_manage")
	// bot.route.manage transitively satisfies bot.route.read per the
	// requireRouteScope OR-semantics in bot_route_service.go.
	ctx := contextWithScopes("paigrambot", "bot.route.manage")

	_, err := svc.GetBotRoute(ctx, &pb.GetBotRouteRequest{
		BotId:    "paigrambot",
		Platform: "telegram",
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	// Route truly does not exist; we should get NotFound, not
	// PermissionDenied — proving the scope check passed.
	assert.Equal(t, codes.NotFound, st.Code())
}
