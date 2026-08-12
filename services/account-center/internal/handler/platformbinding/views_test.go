package platformbinding

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"

	"paigram/internal/model"
)

func TestGrantViewExposesPendingFencePropagation(t *testing.T) {
	view := buildGrantView(&model.ConsumerGrant{
		BindingID: 101, Consumer: "paigram-bot", Status: model.ConsumerGrantStatusRevoked,
		TicketVersion: 2, LastInvalidatedAt: sql.NullTime{},
	})

	assert.Equal(t, "propagation_pending", view.PropagationState)
}

func TestGrantViewOmitsPropagationStateAfterFenceConfirmation(t *testing.T) {
	view := buildGrantView(&model.ConsumerGrant{
		BindingID: 101, Consumer: "paigram-bot", Status: model.ConsumerGrantStatusRevoked,
		TicketVersion: 2, LastInvalidatedAt: sql.NullTime{Valid: true},
	})

	assert.Empty(t, view.PropagationState)
}
