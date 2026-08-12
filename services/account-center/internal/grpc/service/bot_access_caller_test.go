package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/internal/grpc/interceptor"
	"paigram/internal/service/credentials"
)

func TestBotAccessCallerUsesExplicitBotMapping(t *testing.T) {
	ctx := interceptor.WithCredentialClaims(context.Background(), &credentials.AccessClaims{
		ClientID: "telegram-service",
		BotID:    "paigram",
		Scope:    botAccessScopeIssueTicket,
	})

	caller, err := botAccessCallerFromContext(ctx, botAccessScopeIssueTicket)

	require.NoError(t, err)
	require.Equal(t, "paigram", caller.bot.Id)
	require.Equal(t, "telegram-service", caller.consumer)
}
