package model_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
	"paigram/internal/testutil"
)

func TestBotRoute_TableName(t *testing.T) {
	assert.Equal(t, "bot_routes", model.BotRoute{}.TableName())
	assert.Equal(t, "bot_route_audit", model.BotRouteAudit{}.TableName())
}

func TestBotRoute_UpsertIdempotent(t *testing.T) {
	db := testutil.OpenMySQLTestDB(t, "botroute_model", &model.BotRoute{}, &model.BotRouteAudit{})

	first := &model.BotRoute{
		BotID:        "paigrambot",
		Platform:     "telegram",
		ServiceID:    "paigram-genshin-v1",
		Endpoint:     "paigram-genshin:50052",
		HandlersJSON: []byte(`[{"command":"sign"}]`),
		Version:      "1.0.0",
		LastHeartbeatAt: sql.NullTime{
			Time:  time.Now().UTC().Truncate(time.Second),
			Valid: true,
		},
	}
	require.NoError(t, model.UpsertBotRoute(db, first).Error)
	require.NotZero(t, first.ID, "expected primary key to be populated after insert")
	originalID := first.ID

	// Upsert again with the same key but updated payload. The unique key
	// (bot_id, platform) must collapse this into a single row with the new
	// values, not create a second route.
	second := &model.BotRoute{
		BotID:        "paigrambot",
		Platform:     "telegram",
		ServiceID:    "paigram-genshin-v2",
		Endpoint:     "paigram-genshin-blue:50052",
		HandlersJSON: []byte(`[{"command":"sign"},{"command":"help"}]`),
		Version:      "2.0.0",
		LastHeartbeatAt: sql.NullTime{
			Time:  time.Now().UTC().Truncate(time.Second),
			Valid: true,
		},
	}
	require.NoError(t, model.UpsertBotRoute(db, second).Error)

	var rows []model.BotRoute
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1, "expected upsert to collapse to a single row on (bot_id, platform)")

	got := rows[0]
	assert.Equal(t, originalID, got.ID, "upsert must update the existing row, not insert a new one")
	assert.Equal(t, "paigram-genshin-v2", got.ServiceID)
	assert.Equal(t, "paigram-genshin-blue:50052", got.Endpoint)
	assert.Equal(t, "2.0.0", got.Version)
	assert.JSONEq(t, `[{"command":"sign"},{"command":"help"}]`, string(got.HandlersJSON))
	assert.True(t, got.LastHeartbeatAt.Valid)
}

func TestBotRoute_UpsertAllowsDistinctPlatforms(t *testing.T) {
	db := testutil.OpenMySQLTestDB(t, "botroute_model", &model.BotRoute{}, &model.BotRouteAudit{})

	telegram := &model.BotRoute{
		BotID:        "paigrambot",
		Platform:     "telegram",
		ServiceID:    "paigram-genshin",
		Endpoint:     "paigram-genshin:50052",
		HandlersJSON: []byte(`[]`),
		Version:      "1.0.0",
	}
	matrix := &model.BotRoute{
		BotID:        "paigrambot",
		Platform:     "matrix",
		ServiceID:    "paigram-genshin",
		Endpoint:     "paigram-genshin:50052",
		HandlersJSON: []byte(`[]`),
		Version:      "1.0.0",
	}
	require.NoError(t, model.UpsertBotRoute(db, telegram).Error)
	require.NoError(t, model.UpsertBotRoute(db, matrix).Error)

	var count int64
	require.NoError(t, db.Model(&model.BotRoute{}).Where("bot_id = ?", "paigrambot").Count(&count).Error)
	assert.EqualValues(t, 2, count)
}
