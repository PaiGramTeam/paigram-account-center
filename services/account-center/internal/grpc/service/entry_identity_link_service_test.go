package service

import (
	"context"
	"testing"
	"time"

	pb "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"paigram/internal/grpc/interceptor"
	"paigram/internal/service/credentials"
	"paigram/internal/service/entryidentity"
)

type recordingEntryIdentityLinkStarter struct {
	input entryidentity.StartInput
	err   error
}

func (starter *recordingEntryIdentityLinkStarter) Start(_ context.Context, input entryidentity.StartInput) (*entryidentity.StartResult, error) {
	starter.input = input
	if starter.err != nil {
		return nil, starter.err
	}
	return &entryidentity.StartResult{
		ApprovalURL: "https://account.example.com/entry-identity-link#challenge=opaque",
		ChallengeView: entryidentity.ChallengeView{
			Issuer:         "urn:paigram:entry:telegram",
			MaskedSubject:  "12***89",
			BotID:          input.BotID,
			BotDisplayName: "PaiGram",
			ExpiresAt:      time.Date(2026, 8, 13, 12, 5, 0, 0, time.UTC),
		},
	}, nil
}

func TestStartEntryIdentityLinkUsesAuthenticatedPrincipal(t *testing.T) {
	starter := &recordingEntryIdentityLinkStarter{}
	service := (&BotAccessService{}).WithEntryIdentityLinks(starter)
	ctx := interceptor.WithCredentialClaims(context.Background(), &credentials.AccessClaims{
		ClientID: "telegram-service",
		BotID:    "paigrambot",
		Scope:    botAccessScopeLinkIdentity,
	})

	response, err := service.StartEntryIdentityLink(ctx, &pb.StartEntryIdentityLinkRequest{
		ExternalSubject:  "123456789",
		ExternalUsername: "alice",
	})

	require.NoError(t, err)
	assert.Equal(t, "telegram-service", starter.input.Consumer)
	assert.Equal(t, "paigrambot", starter.input.BotID)
	assert.Equal(t, "123456789", starter.input.ExternalSubject)
	assert.Equal(t, "alice", starter.input.ExternalUsername)
	assert.Equal(t, "urn:paigram:entry:telegram", response.GetIssuer())
}

func TestStartEntryIdentityLinkRequiresDedicatedScope(t *testing.T) {
	service := (&BotAccessService{}).WithEntryIdentityLinks(&recordingEntryIdentityLinkStarter{})
	ctx := interceptor.WithCredentialClaims(context.Background(), &credentials.AccessClaims{
		ClientID: "telegram-service",
		BotID:    "paigrambot",
		Scope:    botAccessScopeRead,
	})

	_, err := service.StartEntryIdentityLink(ctx, &pb.StartEntryIdentityLinkRequest{ExternalSubject: "123456789"})

	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestStartEntryIdentityLinkMapsInactiveNamespaceToFailedPrecondition(t *testing.T) {
	service := (&BotAccessService{}).WithEntryIdentityLinks(&recordingEntryIdentityLinkStarter{
		err: entryidentity.ErrNamespaceUnavailable,
	})
	ctx := interceptor.WithCredentialClaims(context.Background(), &credentials.AccessClaims{
		ClientID: "telegram-service", BotID: "paigrambot", Scope: botAccessScopeLinkIdentity,
	})

	_, err := service.StartEntryIdentityLink(ctx, &pb.StartEntryIdentityLinkRequest{ExternalSubject: "123456789"})

	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
