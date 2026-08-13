package platformbinding

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/model"
)

func TestGrantMutationPropagatesPendingEntryEpochWithNewGrantVersion(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	invalidator := &capturingGrantInvalidator{}
	service := NewGrantService(db, invalidator)
	binding := seedGrantServiceBinding(t, db, "cn:entry-epoch-and-grant")
	grant := seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID: binding.ID, Consumer: ConsumerPaiGramBot, Status: model.ConsumerGrantStatusActive,
		TicketVersion: 3, PendingEntryEpoch: 8, GrantedAt: time.Now().UTC(), LastInvalidatedAt: sql.NullTime{},
	}, "mihomo.status.read")

	updated, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID, Consumer: ConsumerPaiGramBot,
		Actions: []string{"mihomo.profile.read"}, GrantedAt: time.Now().UTC(),
	})

	require.NoError(t, err)
	assert.False(t, created)
	require.Equal(t, 1, invalidator.calls)
	assert.Equal(t, uint64(4), invalidator.input.MinimumGrantVersion)
	assert.Equal(t, uint64(8), invalidator.input.MinimumEntryEpoch)
	assert.Equal(t, uint64(4), updated.TicketVersion)
	assert.Zero(t, updated.PendingEntryEpoch)
	assert.True(t, updated.LastInvalidatedAt.Valid)

	var stored model.ConsumerGrant
	require.NoError(t, db.First(&stored, grant.ID).Error)
	assert.Zero(t, stored.PendingEntryEpoch)
}
