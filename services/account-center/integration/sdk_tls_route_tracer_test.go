//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpccredentials "google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/structpb"

	"paigram/internal/config"
	accountgrpc "paigram/internal/grpc/server"
	"paigram/internal/model"
	"paigram/internal/service/credentials"
)

type tracerRuntimeService struct {
	mihomov2.UnimplementedMihomoRuntimeServiceServer
}

func (tracerRuntimeService) DescribePlatform(context.Context, *mihomov2.DescribePlatformRequest) (*mihomov2.DescribePlatformResponse, error) {
	schema, err := structpb.NewStruct(map[string]any{"type": "object"})
	if err != nil {
		return nil, err
	}
	return &mihomov2.DescribePlatformResponse{
		PlatformKey:      "mihomo",
		DisplayName:      "Mihomo",
		ServiceAudience:  "platform-mihomo-runtime",
		SupportedActions: []string{"mihomo.status.read"},
		CredentialSchema: schema,
		ContractVersion:  "v2",
	}, nil
}

func TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners(t *testing.T) {
	stack := newIntegrationStack(t)
	accountTLS := tlstest.NewRSA(t, "account.internal")
	runtimeTLS := tlstest.NewRSA(t, "runtime.internal")
	runtimeAddress := startTracerRuntimeServer(t, runtimeTLS)

	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.Create(&owner).Error)
	credentialResult, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID:    "sdk-tls-tracer",
		BotID:       "sdk-tls-tracer",
		DisplayName: "SDK TLS tracer",
		OwnerUserID: owner.ID,
		Audiences:   []string{"account-center"},
		Scopes:      []string{"bot.access.read", "bot.access.issue_ticket"},
	})
	require.NoError(t, err)
	require.NoError(t, stack.DB.Create(&model.PlatformService{
		PlatformKey:          "mihomo",
		DisplayName:          "Mihomo",
		ServiceKey:           "platform-mihomo-service",
		ServiceAudience:      "platform-mihomo-runtime",
		DiscoveryType:        "static",
		ControlEndpoint:      "control.internal:50051",
		RuntimeEndpoint:      runtimeAddress,
		RuntimeServerName:    runtimeTLS.ServerName,
		Enabled:              true,
		SupportedActionsJSON: `["mihomo.status.read"]`,
		CredentialSchemaJSON: `{}`,
	}).Error)

	accountPort := reserveTCPPort(t)
	accountCfg := newTestConfig(t, uniqueRedisPrefix(t.Name()+"-grpc"))
	accountCfg.GRPC = config.GRPCConfig{
		Enabled:         true,
		Port:            accountPort,
		CertificateFile: accountTLS.ServerCertFile,
		PrivateKeyFile:  accountTLS.ServerKeyFile,
	}
	accountServer, err := accountgrpc.NewGRPCServer(accountPort, stack.DB, stack.Redis, accountCfg)
	require.NoError(t, err)
	serverErr := make(chan error, 1)
	go func() { serverErr <- accountServer.Start() }()
	t.Cleanup(func() {
		accountServer.Stop()
		select {
		case serveErr := <-serverErr:
			require.NoError(t, serveErr)
		case <-time.After(5 * time.Second):
			t.Error("Account gRPC server did not stop")
		}
	})
	waitForTCP(t, fmt.Sprintf("127.0.0.1:%d", accountPort))

	httpServer := httptest.NewServer(stack.Router)
	t.Cleanup(httpServer.Close)
	runPythonTLSRouteTracer(t, pythonTracerInput{
		AccountHTTPURL:    httpServer.URL,
		AccountGRPCTarget: fmt.Sprintf("127.0.0.1:%d", accountPort),
		AccountServerName: accountTLS.ServerName,
		AccountCAFile:     accountTLS.CAFile,
		PlatformCAFile:    runtimeTLS.CAFile,
		ClientID:          credentialResult.ClientID,
		ClientSecret:      credentialResult.ClientSecret,
		ExpectedAudience:  "platform-mihomo-runtime",
		ExpectedActions:   `["mihomo.status.read"]`,
	})
}

func startTracerRuntimeServer(t *testing.T, bundle tlstest.Bundle) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsConfig, err := transporttls.NewServerConfig(transporttls.ServerFiles{
		CertificateFile: bundle.ServerCertFile,
		PrivateKeyFile:  bundle.ServerKeyFile,
	}, transporttls.ServerAuthOnly)
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(grpccredentials.NewTLS(tlsConfig)))
	mihomov2.RegisterMihomoRuntimeServiceServer(server, tracerRuntimeService{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.GracefulStop()
		_ = listener.Close()
	})
	return listener.Addr().String()
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func waitForTCP(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for TCP listener %s", address)
}

type pythonTracerInput struct {
	AccountHTTPURL    string
	AccountGRPCTarget string
	AccountServerName string
	AccountCAFile     string
	PlatformCAFile    string
	ClientID          string
	ClientSecret      string
	ExpectedAudience  string
	ExpectedActions   string
	ExternalUserID    string
	EntrySubject      string
	EntryAction       string
}

func runPythonTLSRouteTracer(t *testing.T, input pythonTracerInput) string {
	t.Helper()
	sdkDirectory, err := filepath.Abs(filepath.Join("..", "..", "..", "sdks", "python"))
	require.NoError(t, err)
	commandContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(commandContext, "uv", "run", "--frozen", "python", "-c", pythonTLSRouteTracer)
	command.Dir = sdkDirectory
	command.Env = append(os.Environ(),
		"PAI_TRACER_HTTP_URL="+input.AccountHTTPURL,
		"PAI_TRACER_ACCOUNT_TARGET="+input.AccountGRPCTarget,
		"PAI_TRACER_ACCOUNT_SERVER_NAME="+input.AccountServerName,
		"PAI_TRACER_ACCOUNT_CA_FILE="+input.AccountCAFile,
		"PAI_TRACER_PLATFORM_CA_FILE="+input.PlatformCAFile,
		"PAI_TRACER_CLIENT_ID="+input.ClientID,
		"PAI_TRACER_CLIENT_SECRET="+input.ClientSecret,
		"PAI_TRACER_EXPECTED_AUDIENCE="+input.ExpectedAudience,
		"PAI_TRACER_EXPECTED_ACTIONS="+input.ExpectedActions,
		"PAI_TRACER_EXTERNAL_USER_ID="+input.ExternalUserID,
		"PAI_TRACER_ENTRY_SUBJECT="+input.EntrySubject,
		"PAI_TRACER_ENTRY_ACTION="+input.EntryAction,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "Python SDK TLS tracer failed:\n%s", output)
	return strings.TrimSpace(string(output))
}

const pythonTLSRouteTracer = `
import asyncio
import json
import os
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from paigram_account_sdk import CredentialStatus, PaiGramAccountClient

async def main():
    async with PaiGramAccountClient(
        account_http_url=os.environ["PAI_TRACER_HTTP_URL"],
        account_grpc_target=os.environ["PAI_TRACER_ACCOUNT_TARGET"],
        account_grpc_server_name=os.environ["PAI_TRACER_ACCOUNT_SERVER_NAME"],
        account_root_certificates=Path(os.environ["PAI_TRACER_ACCOUNT_CA_FILE"]).read_bytes(),
        platform_root_certificates={
            "platform-mihomo-service": Path(os.environ["PAI_TRACER_PLATFORM_CA_FILE"]).read_bytes(),
        },
        client_id=os.environ["PAI_TRACER_CLIENT_ID"],
        client_secret=os.environ["PAI_TRACER_CLIENT_SECRET"],
    ) as client:
        entry_subject = os.environ.get("PAI_TRACER_ENTRY_SUBJECT", "")
        if entry_subject:
            if os.environ.get("PAI_TRACER_ENTRY_ACTION") == "resolve":
                resolved = await client.resolve_user(entry_subject)
                assert resolved.external_user_id == entry_subject
                print(json.dumps({"resolved_bot_id": resolved.bot_id}))
                return
            link = await client.start_entry_identity_link(entry_subject)
            assert link.approval_url.startswith("https://account.example.test/entry-identity-link#challenge=")
            assert link.masked_subject != entry_subject
            challenge = parse_qs(urlparse(link.approval_url).fragment)["challenge"][0]
            print(json.dumps({"challenge": challenge}))
            return
        descriptor = await client.describe_platform("platform-mihomo-service")
        assert descriptor.platform_key == "mihomo"
        assert descriptor.service_audience == os.environ["PAI_TRACER_EXPECTED_AUDIENCE"]
        assert descriptor.supported_actions == tuple(json.loads(os.environ["PAI_TRACER_EXPECTED_ACTIONS"]))
        external_user_id = os.environ.get("PAI_TRACER_EXTERNAL_USER_ID", "")
        if external_user_id:
            bindings = await client.list_bindings(external_user_id, platform="mihomo")
            assert len(bindings) == 1
            assert bindings[0].generation == 1
            status = await client.get_credential_status(external_user_id=external_user_id, binding=bindings[0])
            assert status.status is CredentialStatus.ACTIVE

asyncio.run(main())
`
