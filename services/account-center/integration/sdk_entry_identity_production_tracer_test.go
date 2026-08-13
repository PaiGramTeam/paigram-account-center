//go:build integration

package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	accountv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/account/v1"
	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpccredentials "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"paigram/internal/config"
	servicecredentials "paigram/internal/service/credentials"
)

func decodeEntryChallengeOutput(t *testing.T, output string) string {
	t.Helper()
	var result struct {
		Challenge string `json:"challenge"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &result), output)
	require.NotEmpty(t, result.Challenge)
	return result.Challenge
}

func traceEntryIdentityWebApproval(t *testing.T, stack *integrationStack, accessToken, challenge string) {
	t.Helper()
	preview := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/me/entry-identity-links/preview", map[string]any{
		"challenge": challenge,
	}, authHeaders(accessToken))
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	require.Equal(t, "no-store", preview.Header().Get("Cache-Control"))
	approved := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/me/entry-identity-links/approve", map[string]any{
		"challenge": challenge,
	}, authHeaders(accessToken))
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	replayed := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/me/entry-identity-links/approve", map[string]any{
		"challenge": challenge,
	}, authHeaders(accessToken))
	require.Equal(t, http.StatusConflict, replayed.Code, replayed.Body.String())
	require.Contains(t, replayed.Body.String(), "entry_identity_link_consumed")
}

func traceEntryIdentityConflictAndCancel(
	t *testing.T,
	stack *integrationStack,
	accountHTTPURL string,
	accountPort int,
	accountTLS tlstest.Bundle,
	runtimeTLS tlstest.Bundle,
	credential *servicecredentials.CreateResult,
) {
	t.Helper()
	_, otherAccessToken, _, _, _ := registerAndLogin(t, stack, "production-entry-conflict@example.com", "Password123!")
	baseInput := pythonTracerInput{
		AccountHTTPURL: accountHTTPURL, AccountGRPCTarget: fmt.Sprintf("127.0.0.1:%d", accountPort),
		AccountServerName: accountTLS.ServerName, AccountCAFile: accountTLS.CAFile,
		PlatformCAFile: runtimeTLS.CAFile,
		ClientID:       credential.ClientID, ClientSecret: credential.ClientSecret, EntryAction: "start",
	}
	conflictInput := baseInput
	conflictInput.EntrySubject = "production-entry-subject"
	conflictChallenge := decodeEntryChallengeOutput(t, runPythonTLSRouteTracer(t, conflictInput))
	conflict := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/me/entry-identity-links/approve", map[string]any{
		"challenge": conflictChallenge,
	}, authHeaders(otherAccessToken))
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
	require.Contains(t, conflict.Body.String(), "entry_identity_link_conflict")

	cancelInput := baseInput
	cancelInput.EntrySubject = "production-entry-cancel-subject"
	cancelChallenge := decodeEntryChallengeOutput(t, runPythonTLSRouteTracer(t, cancelInput))
	cancelled := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/me/entry-identity-links/cancel", map[string]any{
		"challenge": cancelChallenge,
	}, authHeaders(otherAccessToken))
	require.Equal(t, http.StatusNoContent, cancelled.Code, cancelled.Body.String())
	replayed := performJSONRequest(t, stack.Router, http.MethodPost, "/api/v1/me/entry-identity-links/cancel", map[string]any{
		"challenge": cancelChallenge,
	}, authHeaders(otherAccessToken))
	require.Equal(t, http.StatusConflict, replayed.Code, replayed.Body.String())
	require.Contains(t, replayed.Body.String(), "entry_identity_link_consumed")
}

func issueProductionEntryTicket(
	t *testing.T,
	stack *integrationStack,
	cfg *config.Config,
	accountPort int,
	accountTLS tlstest.Bundle,
	credential *servicecredentials.CreateResult,
	bindingRef string,
	externalSubject string,
) string {
	t.Helper()
	tokens, err := servicecredentials.NewTokenService(servicecredentials.NewService(stack.DB), servicecredentials.TokenServiceConfig{
		Issuer: cfg.Auth.OAuthIssuer, AccessTokenTTLSeconds: cfg.Auth.OAuthAccessTokenTTLSeconds,
		SigningKey: []byte(cfg.Auth.OAuthSigningKey),
	})
	require.NoError(t, err)
	issued, err := tokens.IssueClientCredentials(servicecredentials.IssueClientCredentialsInput{
		ClientID: credential.ClientID, ClientSecret: credential.ClientSecret, Audience: "account-center",
		RequestedScopes: []string{"bot.access.read", "bot.access.issue_ticket"},
	})
	require.NoError(t, err)
	connection := dialProductionTLS(t, fmt.Sprintf("127.0.0.1:%d", accountPort), accountTLS)
	client := accountv1.NewBotAccessServiceClient(connection)
	authorized := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+issued.AccessToken)
	resolved, err := client.ResolveBotUser(authorized, &accountv1.ResolveBotUserRequest{ExternalUserId: externalSubject})
	require.NoError(t, err)
	require.Equal(t, credential.Credential.BotID, resolved.BotId)
	ticket, err := client.IssueServiceTicket(authorized, &accountv1.IssueServiceTicketRequest{
		ExternalUserId: externalSubject, BindingRef: bindingRef, RequestedAction: platformaction.MihomoStatusRead,
	})
	require.NoError(t, err)
	require.NotEmpty(t, ticket.Ticket)
	return ticket.Ticket
}

func callProductionRuntimeStatus(
	ctx context.Context,
	address string,
	bundle tlstest.Bundle,
	ticket string,
	bindingRef string,
	accountKey string,
) error {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bundle.CACertificatePEM) {
		return fmt.Errorf("append runtime CA certificate")
	}
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(grpccredentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: bundle.ServerName,
	})))
	if err != nil {
		return err
	}
	defer connection.Close()
	authorized := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+ticket)
	_, err = mihomov2.NewMihomoRuntimeServiceClient(connection).GetStatus(authorized, &mihomov2.GetStatusRequest{
		Resource: &platformv2.BindingResource{BindingRef: bindingRef, AccountKey: accountKey},
	})
	return err
}

func dialProductionTLS(t *testing.T, address string, bundle tlstest.Bundle) *grpc.ClientConn {
	t.Helper()
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(bundle.CACertificatePEM))
	connection, err := grpc.NewClient(address, grpc.WithTransportCredentials(grpccredentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: bundle.ServerName,
	})))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return connection
}
