//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/platformaction"
	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"paigram/integration/testenv"
	"paigram/internal/config"
	accountgrpc "paigram/internal/grpc/server"
	"paigram/internal/model"
	"paigram/internal/platformtransport"
	"paigram/internal/service/credentials"
	"paigram/internal/service/platform"
	"paigram/internal/service/platformbinding"
	"paigram/internal/testutil"
)

func TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket(t *testing.T) {
	stack := newIntegrationStack(t)
	controlTLS := tlstest.NewRSA(t, "platform-control.internal")
	runtimeTLS := tlstest.NewRSA(t, "platform-runtime.internal")
	accountTLS := tlstest.NewRSA(t, "account.internal")
	privateKeyPEM, publicKeyPEM, err := contractticket.GenerateKeyPairPEM()
	require.NoError(t, err)
	signingKeyFile := testutil.WriteServiceTicketSigningKey(t, "production-tracer", privateKeyPEM)
	publicKeyringFile := writeProductionTracerJSON(t, contractticket.PublicKeyringFile{Keys: []contractticket.PublicKeyEntry{{
		KeyID: "production-tracer", PublicKeyPEM: publicKeyPEM,
	}}})
	encryptionKeyringFile := writeProductionTracerJSON(t, map[string]any{
		"active_kid": "production-tracer",
		"keys": []map[string]string{{
			"kid": "production-tracer", "key_base64": base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		}},
	})

	upstream := newProductionTracerUpstream(t)
	controlPort := reserveTCPPort(t)
	runtimePort := reserveTCPPort(t)
	platformDSN := newProductionTracerDatabase(t)
	platformConfig := writeProductionPlatformConfig(t, productionPlatformConfig{
		ControlPort: controlPort, RuntimePort: runtimePort, ControlTLS: controlTLS, RuntimeTLS: runtimeTLS,
		DatabaseDSN: platformDSN, RedisAddress: testenv.MustShared().RedisAddr,
		RedisPrefix: uniqueRedisPrefix(t.Name() + "-platform"), EncryptionKeyringFile: encryptionKeyringFile,
		PublicKeyringFile: publicKeyringFile, UpstreamURL: upstream.URL,
	})
	startProductionPlatform(t, platformConfig, controlPort, runtimePort)

	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, stack.DB.Create(&owner).Error)
	credentialResult, err := credentials.NewService(stack.DB).Create(credentials.CreateInput{
		ClientID: "sdk-production-tracer", BotID: "sdk-production-tracer", DisplayName: "SDK production tracer",
		OwnerUserID: owner.ID, Audiences: []string{"account-center"},
		Scopes: []string{"bot.access.read", "bot.access.issue_ticket"},
	})
	require.NoError(t, err)
	externalUserID := "telegram:production-tracer"
	require.NoError(t, stack.DB.Create(&model.BotIdentity{
		UserID: owner.ID, BotID: credentialResult.Credential.BotID, ExternalUserID: externalUserID,
	}).Error)

	actions := append(platformaction.MihomoDelegationActions(), platformaction.MihomoControlActions()...)
	actionsJSON, err := json.Marshal(actions)
	require.NoError(t, err)
	controlAddress := fmt.Sprintf("127.0.0.1:%d", controlPort)
	runtimeAddress := fmt.Sprintf("127.0.0.1:%d", runtimePort)
	require.NoError(t, stack.DB.Create(&model.PlatformService{
		PlatformKey: "mihomo", DisplayName: "Mihomo", ServiceKey: "platform-mihomo-service",
		ServiceAudience: "platform-mihomo-service", DiscoveryType: "static", ControlEndpoint: controlAddress,
		RuntimeEndpoint: runtimeAddress, RuntimeServerName: runtimeTLS.ServerName, Enabled: true,
		SupportedActionsJSON: string(actionsJSON), CredentialSchemaJSON: `{}`,
	}).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID: owner.ID, Platform: "mihomo", PlatformServiceKey: "platform-mihomo-service",
		DisplayName: "Production tracer account", Status: model.PlatformAccountBindingStatusPendingBind,
	}
	require.NoError(t, stack.DB.Create(&binding).Error)

	accountCfg := newTestConfig(t, uniqueRedisPrefix(t.Name()+"-account-grpc"))
	accountCfg.Auth.ServiceTicketSigningKeyFile = signingKeyFile
	accountCfg.PlatformControl = config.PlatformControlConfig{
		RootCAFile: controlTLS.CAFile, CertificateFile: controlTLS.ClientCertFile,
		PrivateKeyFile: controlTLS.ClientKeyFile, ServerName: controlTLS.ServerName, DialTimeout: 5 * time.Second,
	}
	controlDialer, err := platformtransport.NewControlDialer(platformtransport.ControlConfig{
		RootCAFile: controlTLS.CAFile, CertificateFile: controlTLS.ClientCertFile,
		PrivateKeyFile: controlTLS.ClientKeyFile, ServerName: controlTLS.ServerName, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	platformService := platform.NewServiceGroup(stack.DB)
	require.NoError(t, platformService.PlatformService.ConfigureAuth(accountCfg.Auth))
	operationID, err := operationid.NewID()
	require.NoError(t, err)
	ticket, _, err := platformService.PlatformService.IssueBindingScopedOperationTicket(
		"user", owner.UserRef, &binding, operationID, []string{platformaction.MihomoCredentialBind},
	)
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]string{
		"cookie_bundle": `{"cookie_token":"production-tracer"}`,
		"device_id":     "production-device", "device_fp": "production-fingerprint", "device_name": "Tracer",
	})
	require.NoError(t, err)
	summary, err := platformbinding.NewGRPCGenericCredentialGateway(controlDialer).BindCredential(
		context.Background(), controlAddress, ticket, operationID, &binding, payload,
	)
	require.NoError(t, err)
	accountKey, ok := summary["platform_account_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, accountKey)
	require.NoError(t, stack.DB.Model(&binding).Updates(map[string]any{
		"external_account_key": accountKey, "generation": uint64(1), "status": model.PlatformAccountBindingStatusActive,
	}).Error)
	binding.ExternalAccountKey = sql.NullString{String: accountKey, Valid: true}
	binding.Generation = 1
	binding.Status = model.PlatformAccountBindingStatusActive
	_, _, err = platformbinding.NewGrantService(stack.DB).UpsertGrant(platformbinding.UpsertGrantInput{
		Context: context.Background(), BindingID: binding.ID, Consumer: credentialResult.ClientID,
		Actions: []string{platformaction.MihomoStatusRead}, GrantedBy: sql.NullInt64{Int64: int64(owner.ID), Valid: true},
		GrantedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	accountPort := reserveTCPPort(t)
	accountCfg.GRPC = config.GRPCConfig{
		Enabled: true, Port: accountPort, CertificateFile: accountTLS.ServerCertFile, PrivateKeyFile: accountTLS.ServerKeyFile,
	}
	accountServer, err := accountgrpc.NewGRPCServer(accountPort, stack.DB, stack.Redis, accountCfg)
	require.NoError(t, err)
	accountServerErr := make(chan error, 1)
	go func() { accountServerErr <- accountServer.Start() }()
	t.Cleanup(func() {
		accountServer.Stop()
		select {
		case serveErr := <-accountServerErr:
			require.NoError(t, serveErr)
		case <-time.After(5 * time.Second):
			t.Error("Account gRPC server did not stop")
		}
	})
	waitForTCP(t, fmt.Sprintf("127.0.0.1:%d", accountPort))

	accountHTTP := httptest.NewServer(stack.Router)
	t.Cleanup(accountHTTP.Close)
	runPythonTLSRouteTracer(t, pythonTracerInput{
		AccountHTTPURL: accountHTTP.URL, AccountGRPCTarget: fmt.Sprintf("127.0.0.1:%d", accountPort),
		AccountServerName: accountTLS.ServerName, AccountCAFile: accountTLS.CAFile,
		PlatformCAFile: runtimeTLS.CAFile, ClientID: credentialResult.ClientID, ClientSecret: credentialResult.ClientSecret,
		ExpectedAudience: "platform-mihomo-service", ExpectedActions: string(actionsJSON), ExternalUserID: externalUserID,
	})
}

type productionPlatformConfig struct {
	ControlPort, RuntimePort           int
	ControlTLS, RuntimeTLS             tlstest.Bundle
	DatabaseDSN, RedisAddress          string
	RedisPrefix, EncryptionKeyringFile string
	PublicKeyringFile, UpstreamURL     string
}

func writeProductionPlatformConfig(t *testing.T, input productionPlatformConfig) string {
	t.Helper()
	quote := func(value string) string {
		raw, err := json.Marshal(filepath.ToSlash(value))
		require.NoError(t, err)
		return string(raw)
	}
	configBody := fmt.Sprintf(`server:
  control:
    network: tcp
    addr: "127.0.0.1:%d"
    timeout_seconds: 5
    tls:
      certificate_file: %s
      private_key_file: %s
      client_ca_file: %s
  runtime:
    network: tcp
    addr: "127.0.0.1:%d"
    timeout_seconds: 5
    tls:
      certificate_file: %s
      private_key_file: %s
data:
  database:
    dsn: %s
  redis:
    addr: %s
    db: 0
    prefix: %s
security:
  credential_encryption_keyring_file: %s
  service_ticket_issuer: paigram-account-center
  service_ticket_public_keyring_file: %s
upstream:
  base_url: %s
  timeout_seconds: 5
  allow_insecure_http: true
`, input.ControlPort, quote(input.ControlTLS.ServerCertFile), quote(input.ControlTLS.ServerKeyFile), quote(input.ControlTLS.CAFile),
		input.RuntimePort, quote(input.RuntimeTLS.ServerCertFile), quote(input.RuntimeTLS.ServerKeyFile), quote(input.DatabaseDSN),
		quote(input.RedisAddress), quote(input.RedisPrefix), quote(input.EncryptionKeyringFile), quote(input.PublicKeyringFile), quote(input.UpstreamURL))
	path := filepath.Join(t.TempDir(), "platform-production-tracer.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configBody), 0o600))
	return path
}

func writeProductionTracerJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "production-tracer.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func newProductionTracerDatabase(t *testing.T) string {
	t.Helper()
	shared := testenv.MustShared()
	name := fmt.Sprintf("platform_tracer_%d", time.Now().UTC().UnixNano())
	admin, err := sql.Open("pgx", shared.PostgresAdminDSN)
	require.NoError(t, err)
	_, err = admin.ExecContext(context.Background(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	require.NoError(t, admin.Close())

	parsed, err := pgx.ParseConfig(shared.PostgresAdminDSN)
	require.NoError(t, err)
	parsed.Database = name
	t.Cleanup(func() {
		cleanup, openErr := sql.Open("pgx", shared.PostgresAdminDSN)
		if openErr != nil {
			t.Errorf("open PostgreSQL admin connection: %v", openErr)
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1", name)
		if _, dropErr := cleanup.ExecContext(context.Background(), "DROP DATABASE "+name); dropErr != nil {
			t.Errorf("drop production tracer database: %v", dropErr)
		}
	})
	return parsed.ConnString()
}

func startProductionPlatform(t *testing.T, configPath string, controlPort, runtimePort int) {
	t.Helper()
	platformRoot, err := filepath.Abs(filepath.Join("..", "..", "platform-mihomo"))
	require.NoError(t, err)
	binaryName := "platform-mihomo-service"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	buildContext, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildContext, "go", "build", "-o", binaryPath, "./cmd/platform-mihomo-service")
	build.Dir = platformRoot
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, "build production Platform binary:\n%s", buildOutput)

	logPath := filepath.Join(t.TempDir(), "platform.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	command := exec.Command(binaryPath, "-conf", configPath)
	command.Dir = platformRoot
	command.Stdout = logFile
	command.Stderr = logFile
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
		_ = logFile.Close()
	})
	waitForProductionPlatform(t, fmt.Sprintf("127.0.0.1:%d", controlPort), logPath)
	waitForProductionPlatform(t, fmt.Sprintf("127.0.0.1:%d", runtimePort), logPath)
}

func waitForProductionPlatform(t *testing.T, address, logPath string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := netDialTimeout(address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	logOutput, _ := os.ReadFile(logPath)
	t.Fatalf("timed out waiting for production Platform listener %s:\n%s", address, logOutput)
}

func newProductionTracerUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/credentials:discover" {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account_id": "10001", "region": "cn_gf01",
			"profiles": []map[string]any{{
				"GameBiz": "hk4e_cn", "Region": "cn_gf01", "PlayerID": "1008611",
				"Nickname": "Traveler", "Level": 60,
			}},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func netDialTimeout(address string, timeout time.Duration) (net.Conn, error) {
	return (&net.Dialer{Timeout: timeout}).Dial("tcp", address)
}
