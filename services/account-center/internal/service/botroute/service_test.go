package botroute_test

import (
	"context"
	"errors"
	"testing"

	botv1 "github.com/PaiGramTeam/proto-contracts/bot/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pb "paigram/internal/grpc/pb/v1"
	"paigram/internal/model"
	"paigram/internal/service/botroute"
	"paigram/internal/testutil"
)

func setupBotRouteServiceTestDB(t *testing.T, prefix string) *gorm.DB {
	t.Helper()
	return testutil.OpenPostgreSQLTestDB(t, prefix, &model.BotRoute{}, &model.BotRouteAudit{})
}

func newTestService(db *gorm.DB) *botroute.Service {
	return botroute.NewService(db, nil)
}

func TestService_RegisterCreatesFreshRoute(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_register_fresh")
	svc := newTestService(db)
	ctx := botroute.WithActor(context.Background(), "paigrambot")

	resp, err := svc.Register(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
		Endpoint:  "paigram-genshin:50052",
		Version:   "1.0.0",
		Handlers: []*botv1.HandlerDeclaration{
			{Command: "sign", Description: "daily sign-in", Visibility: botv1.HandlerVisibility_HANDLER_VISIBILITY_PUBLIC},
			{Command: "debug", Description: "internal", Visibility: botv1.HandlerVisibility_HANDLER_VISIBILITY_HIDDEN},
		},
	})
	require.NoError(t, err)
	assert.NotZero(t, resp.GetRegisteredAtUnix())

	var rows []model.BotRoute
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "paigram-genshin", rows[0].ServiceID)
	assert.Equal(t, "paigram-genshin:50052", rows[0].Endpoint)
	assert.True(t, rows[0].LastHeartbeatAt.Valid)
}

func TestService_RegisterOverwritesExistingRoute(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_register_overwrite")
	svc := newTestService(db)
	ctx := botroute.WithActor(context.Background(), "paigrambot")

	_, err := svc.Register(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
		Endpoint:  "paigram-genshin:50052",
		Version:   "1.0.0",
	})
	require.NoError(t, err)

	_, err = svc.Register(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin-v2",
		Endpoint:  "paigram-genshin-blue:50052",
		Version:   "2.0.0",
	})
	require.NoError(t, err)

	var rows []model.BotRoute
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1, "second register must collapse onto existing (bot_id, platform) row")
	assert.Equal(t, "paigram-genshin-v2", rows[0].ServiceID)
	assert.Equal(t, "paigram-genshin-blue:50052", rows[0].Endpoint)
	assert.Equal(t, "2.0.0", rows[0].Version)
}

func TestService_RegisterValidatesRequiredFields(t *testing.T) {
	// Validation must reject malformed requests *before* touching the
	// database, so we deliberately pass a nil db here to guarantee that a
	// regression that delays validation behind a db call would surface as
	// a panic rather than a silent pass.
	svc := botroute.NewService(nil, nil)

	cases := []struct {
		name string
		req  *pb.RegisterBotServiceRequest
	}{
		{"missing bot_id", &pb.RegisterBotServiceRequest{Platform: "telegram", ServiceId: "s", Endpoint: "e"}},
		{"missing platform", &pb.RegisterBotServiceRequest{BotId: "b", ServiceId: "s", Endpoint: "e"}},
		{"missing service_id", &pb.RegisterBotServiceRequest{BotId: "b", Platform: "telegram", Endpoint: "e"}},
		{"missing endpoint", &pb.RegisterBotServiceRequest{BotId: "b", Platform: "telegram", ServiceId: "s"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), tc.req)
			require.Error(t, err)
			assert.ErrorIs(t, err, botroute.ErrInvalidRouteRequest)
		})
	}
}

func TestService_RegisterWritesAuditEntry(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_register_audit")
	svc := newTestService(db)
	ctx := botroute.WithActor(context.Background(), "paigrambot")

	_, err := svc.Register(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
		Endpoint:  "paigram-genshin:50052",
		Version:   "1.0.0",
	})
	require.NoError(t, err)

	var audits []model.BotRouteAudit
	require.NoError(t, db.Order("id ASC").Find(&audits).Error)
	require.Len(t, audits, 1)
	assert.Equal(t, "register", audits[0].Action)
	assert.Equal(t, "paigrambot", audits[0].BotID)
	assert.Equal(t, "telegram", audits[0].Platform)
	assert.Equal(t, "paigrambot", audits[0].Actor)
	assert.NotEmpty(t, audits[0].Payload)
}

func TestService_UnregisterReturnsNotFoundForUnknownRoute(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_unreg_missing")
	svc := newTestService(db)

	_, err := svc.Unregister(context.Background(), &pb.UnregisterBotServiceRequest{
		BotId:     "ghost",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, botroute.ErrRouteNotFound))
}

func TestService_UnregisterRejectsServiceIDMismatch(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_unreg_mismatch")
	svc := newTestService(db)
	ctx := botroute.WithActor(context.Background(), "paigrambot")

	_, err := svc.Register(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin-v2",
		Endpoint:  "paigram-genshin-blue:50052",
		Version:   "2.0.0",
	})
	require.NoError(t, err)

	// Late shutdown from v1 must NOT be allowed to release a route already
	// owned by v2.
	_, err = svc.Unregister(ctx, &pb.UnregisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin-v1",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, botroute.ErrServiceIDMismatch))

	// And the row must still exist.
	var count int64
	require.NoError(t, db.Model(&model.BotRoute{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestService_GetRouteSuccess(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_get_ok")
	svc := newTestService(db)
	ctx := botroute.WithActor(context.Background(), "paigrambot")

	_, err := svc.Register(ctx, &pb.RegisterBotServiceRequest{
		BotId:     "paigrambot",
		Platform:  "telegram",
		ServiceId: "paigram-genshin",
		Endpoint:  "paigram-genshin:50052",
		Version:   "1.0.0",
		Handlers: []*botv1.HandlerDeclaration{
			{Command: "sign", Description: "daily sign-in", Visibility: botv1.HandlerVisibility_HANDLER_VISIBILITY_PUBLIC},
		},
	})
	require.NoError(t, err)

	resp, err := svc.GetRoute(context.Background(), &pb.GetBotRouteRequest{
		BotId:    "paigrambot",
		Platform: "telegram",
	})
	require.NoError(t, err)
	assert.Equal(t, "paigram-genshin", resp.GetServiceId())
	assert.Equal(t, "paigram-genshin:50052", resp.GetEndpoint())
	assert.Equal(t, "1.0.0", resp.GetVersion())
	require.Len(t, resp.GetHandlers(), 1)
	assert.Equal(t, "sign", resp.GetHandlers()[0].GetCommand())
	assert.Equal(t, botv1.HandlerVisibility_HANDLER_VISIBILITY_PUBLIC, resp.GetHandlers()[0].GetVisibility())
	assert.NotZero(t, resp.GetLastHeartbeatAtUnix())
}

func TestService_GetRouteReturnsNotFound(t *testing.T) {
	db := setupBotRouteServiceTestDB(t, "botroute_get_missing")
	svc := newTestService(db)

	_, err := svc.GetRoute(context.Background(), &pb.GetBotRouteRequest{
		BotId:    "ghost",
		Platform: "telegram",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, botroute.ErrRouteNotFound))
}
