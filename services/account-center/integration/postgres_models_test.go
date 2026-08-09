//go:build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/integration/testenv"
	"paigram/internal/model"
)

func TestPostgreSQLModelTypesMatchMigratedSchema(t *testing.T) {
	database := testenv.NewPerTestDB(t)
	require.NoError(t, database.GORM.AutoMigrate(
		&model.LegalDocument{},
		&model.BotRoute{},
		&model.BotRouteAudit{},
	))

	route := model.BotRoute{
		BotID:        "model-contract-bot",
		Platform:     "genshin",
		ServiceID:    "genshin-service",
		Endpoint:     "http://genshin-service:8080",
		HandlersJSON: []byte(`[{"command":"help"}]`),
		Version:      "1",
	}
	require.NoError(t, model.UpsertBotRoute(database.GORM, &route).Error)

	var stored model.BotRoute
	require.NoError(t, database.GORM.First(&stored, route.ID).Error)
	require.JSONEq(t, string(route.HandlersJSON), string(stored.HandlersJSON))
}
