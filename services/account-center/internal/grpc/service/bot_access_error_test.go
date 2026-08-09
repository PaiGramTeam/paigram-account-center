package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"paigram/internal/service/botaccess"
)

func TestMapBotAccessErrorDistinguishesBindingStateFromServiceAvailability(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "inactive binding", err: botaccess.ErrInactiveAccountRef, code: codes.FailedPrecondition},
		{name: "invalid ticket config", err: botaccess.ErrInvalidTicketConfig, code: codes.Unavailable},
		{name: "missing signing key", err: botaccess.ErrSigningKeyUnavailable, code: codes.Unavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped := mapBotAccessError("issue service ticket", test.err)
			assert.Equal(t, test.code, status.Code(mapped))
		})
	}
}
