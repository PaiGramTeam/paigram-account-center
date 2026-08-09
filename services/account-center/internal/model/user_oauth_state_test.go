package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"paigram/internal/model"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserOAuthState{}))
	return db
}

func TestUserOAuthState_MetadataRoundTrip(t *testing.T) {
	db := newTestDB(t)
	meta, err := json.Marshal(map[string]string{"bot_id": "paigrambot"})
	require.NoError(t, err)

	row := model.UserOAuthState{
		Provider:     "telegram",
		Purpose:      "telegram_oidc",
		State:        "abc123",
		CodeVerifier: "verifier",
		ClientIP:     "1.2.3.4",
		UserAgent:    "ua",
		Metadata:     datatypes.JSON(meta),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	require.NoError(t, db.Create(&row).Error)

	var fetched model.UserOAuthState
	require.NoError(t, db.Where("state = ?", "abc123").First(&fetched).Error)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal(fetched.Metadata, &parsed))
	assert.Equal(t, "paigrambot", parsed["bot_id"])
}

func TestUserOAuthState_ConsumedAtNullByDefault(t *testing.T) {
	db := newTestDB(t)
	row := model.UserOAuthState{
		Provider:  "telegram",
		Purpose:   "telegram_oidc",
		State:     "xyz789",
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	require.NoError(t, db.Create(&row).Error)

	var fetched model.UserOAuthState
	require.NoError(t, db.Where("state = ?", "xyz789").First(&fetched).Error)
	assert.False(t, fetched.ConsumedAt.Valid, "consumed_at must be NULL on fresh rows")
}
