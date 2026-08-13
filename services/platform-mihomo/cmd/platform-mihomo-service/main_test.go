package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/glebarez/sqlite"
	kratosconfig "github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"platform-mihomo-service/internal/conf"
	internalcrypto "platform-mihomo-service/internal/crypto"
)

const mainTestServiceTicketKeyID = "main-test-key"

var mainTestServiceTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))

func TestValidateBootstrap(t *testing.T) {
	t.Run("accepts valid bootstrap", func(t *testing.T) {
		bc := validMainTestBootstrap(t)

		if err := validateBootstrap(bc); err != nil {
			t.Fatalf("validateBootstrap() error = %v", err)
		}
	})

	t.Run("rejects missing PostgreSQL database DSN", func(t *testing.T) {
		bc := validMainTestBootstrap(t)
		bc.Data.Database.Dsn = ""

		err := validateBootstrap(bc)
		require.EqualError(t, err, "data.database.dsn is required")
	})

	t.Run("rejects missing Redis address", func(t *testing.T) {
		bc := validMainTestBootstrap(t)
		bc.Data.Redis.Addr = ""

		require.EqualError(t, validateBootstrap(bc), "data.redis.addr is required")
	})

	t.Run("rejects missing metrics address", func(t *testing.T) {
		bc := validMainTestBootstrap(t)
		bc.Metrics.Addr = ""

		require.EqualError(t, validateBootstrap(bc), "metrics.addr is required")
	})

	t.Run("rejects missing grpc address", func(t *testing.T) {
		bc := validMainTestBootstrap(t)
		bc.Server.Control.Addr = ""

		require.EqualError(t, validateBootstrap(bc), "server.control.addr is required")
	})

	t.Run("rejects missing service ticket public keyring file", func(t *testing.T) {
		bc := validMainTestBootstrap(t)
		bc.Security.ServiceTicketPublicKeyringFile = ""

		if err := validateBootstrap(bc); err == nil {
			t.Fatal("validateBootstrap() error = nil, want non-nil")
		}
	})
}

func TestLoadBootstrapSecretFilesOverridesInlineValues(t *testing.T) {
	directory := t.TempDir()
	dsnFile := filepath.Join(directory, "database-dsn")
	passwordFile := filepath.Join(directory, "redis-password")
	require.NoError(t, os.WriteFile(dsnFile, []byte("postgres://secret\n"), 0o600))
	require.NoError(t, os.WriteFile(passwordFile, []byte("redis-secret\n"), 0o600))
	bootstrap := &conf.Bootstrap{Data: &conf.Data{
		Database: &conf.Data_Database{Dsn: "inline", DsnFile: dsnFile},
		Redis:    &conf.Data_Redis{Password: "inline", PasswordFile: passwordFile},
	}}
	require.NoError(t, loadBootstrapSecretFiles(bootstrap))
	require.Equal(t, "postgres://secret", bootstrap.Data.Database.Dsn)
	require.Equal(t, "redis-secret", bootstrap.Data.Redis.Password)
}

func TestUpstreamBaseURLResolvesFromPrefixedEnvironment(t *testing.T) {
	t.Setenv("PAI_UPSTREAM_BASE_URL", "https://mihomo.example.test")
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("upstream:\n  base_url: ${UPSTREAM_BASE_URL}\n"), 0o600))

	loaded := kratosconfig.New(kratosconfig.WithSource(file.NewSource(path), env.NewSource("PAI_")))
	t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	require.NoError(t, loaded.Load())
	var bootstrap conf.Bootstrap
	require.NoError(t, loaded.Scan(&bootstrap))
	require.Equal(t, "https://mihomo.example.test", bootstrap.GetUpstream().GetBaseUrl())
}

func TestSecretFilePathsResolveFromPrefixedEnvironment(t *testing.T) {
	values := map[string]string{
		"PAI_DATA_DATABASE_DSN_FILE":                      "/run/secrets/platform-database-dsn",
		"PAI_DATA_REDIS_PASSWORD_FILE":                    "/run/secrets/platform-redis-password",
		"PAI_SECURITY_CREDENTIAL_ENCRYPTION_KEYRING_FILE": "/run/secrets/platform-encryption-keyring",
		"PAI_SECURITY_SERVICE_TICKET_PUBLIC_KEYRING_FILE": "/run/secrets/account-ticket-keyring",
		"PAI_SERVER_CONTROL_TLS_CERTIFICATE_FILE":         "/run/secrets/platform-control-cert",
		"PAI_SERVER_CONTROL_TLS_PRIVATE_KEY_FILE":         "/run/secrets/platform-control-key",
		"PAI_SERVER_CONTROL_TLS_CLIENT_CA_FILE":           "/run/secrets/account-client-ca",
		"PAI_SERVER_RUNTIME_TLS_CERTIFICATE_FILE":         "/run/secrets/platform-runtime-cert",
		"PAI_SERVER_RUNTIME_TLS_PRIVATE_KEY_FILE":         "/run/secrets/platform-runtime-key",
		"PAI_UPSTREAM_BEARER_TOKEN_FILE":                  "/run/secrets/upstream-token",
		"PAI_UPSTREAM_ROOT_CA_FILE":                       "/run/secrets/upstream-ca",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	configuration := `
server:
  control:
    tls:
      certificate_file: "${SERVER_CONTROL_TLS_CERTIFICATE_FILE}"
      private_key_file: "${SERVER_CONTROL_TLS_PRIVATE_KEY_FILE}"
      client_ca_file: "${SERVER_CONTROL_TLS_CLIENT_CA_FILE}"
  runtime:
    tls:
      certificate_file: "${SERVER_RUNTIME_TLS_CERTIFICATE_FILE}"
      private_key_file: "${SERVER_RUNTIME_TLS_PRIVATE_KEY_FILE}"
data:
  database:
    dsn_file: "${DATA_DATABASE_DSN_FILE}"
  redis:
    password_file: "${DATA_REDIS_PASSWORD_FILE}"
security:
  credential_encryption_keyring_file: "${SECURITY_CREDENTIAL_ENCRYPTION_KEYRING_FILE}"
  service_ticket_public_keyring_file: "${SECURITY_SERVICE_TICKET_PUBLIC_KEYRING_FILE}"
upstream:
  bearer_token_file: "${UPSTREAM_BEARER_TOKEN_FILE}"
  root_ca_file: "${UPSTREAM_ROOT_CA_FILE}"
`
	require.NoError(t, os.WriteFile(path, []byte(configuration), 0o600))

	loaded := kratosconfig.New(kratosconfig.WithSource(file.NewSource(path), env.NewSource("PAI_")))
	t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	require.NoError(t, loaded.Load())
	var bootstrap conf.Bootstrap
	require.NoError(t, loaded.Scan(&bootstrap))
	require.Equal(t, values["PAI_DATA_DATABASE_DSN_FILE"], bootstrap.GetData().GetDatabase().GetDsnFile())
	require.Equal(t, values["PAI_DATA_REDIS_PASSWORD_FILE"], bootstrap.GetData().GetRedis().GetPasswordFile())
	require.Equal(t, values["PAI_SECURITY_CREDENTIAL_ENCRYPTION_KEYRING_FILE"], bootstrap.GetSecurity().GetCredentialEncryptionKeyringFile())
	require.Equal(t, values["PAI_SECURITY_SERVICE_TICKET_PUBLIC_KEYRING_FILE"], bootstrap.GetSecurity().GetServiceTicketPublicKeyringFile())
	require.Equal(t, values["PAI_SERVER_CONTROL_TLS_CERTIFICATE_FILE"], bootstrap.GetServer().GetControl().GetTls().GetCertificateFile())
	require.Equal(t, values["PAI_SERVER_CONTROL_TLS_PRIVATE_KEY_FILE"], bootstrap.GetServer().GetControl().GetTls().GetPrivateKeyFile())
	require.Equal(t, values["PAI_SERVER_CONTROL_TLS_CLIENT_CA_FILE"], bootstrap.GetServer().GetControl().GetTls().GetClientCaFile())
	require.Equal(t, values["PAI_SERVER_RUNTIME_TLS_CERTIFICATE_FILE"], bootstrap.GetServer().GetRuntime().GetTls().GetCertificateFile())
	require.Equal(t, values["PAI_SERVER_RUNTIME_TLS_PRIVATE_KEY_FILE"], bootstrap.GetServer().GetRuntime().GetTls().GetPrivateKeyFile())
	require.Equal(t, values["PAI_UPSTREAM_BEARER_TOKEN_FILE"], bootstrap.GetUpstream().GetBearerTokenFile())
	require.Equal(t, values["PAI_UPSTREAM_ROOT_CA_FILE"], bootstrap.GetUpstream().GetRootCaFile())
}

func validMainTestBootstrap(t *testing.T) *conf.Bootstrap {
	t.Helper()
	controlTLS := tlstest.New(t, "control.internal")
	runtimeTLS := tlstest.New(t, "runtime.internal")
	return &conf.Bootstrap{
		Server: &conf.Server{
			Control: &conf.Server_GRPC{
				Network:        "tcp",
				Addr:           "127.0.0.1:9000",
				TimeoutSeconds: 5,
				Tls: &conf.Server_TLS{
					CertificateFile: controlTLS.ServerCertFile,
					PrivateKeyFile:  controlTLS.ServerKeyFile,
					ClientCaFile:    controlTLS.CAFile,
				},
			},
			Runtime: &conf.Server_GRPC{
				Network:        "tcp",
				Addr:           "127.0.0.1:9001",
				TimeoutSeconds: 5,
				Tls: &conf.Server_TLS{
					CertificateFile: runtimeTLS.ServerCertFile,
					PrivateKeyFile:  runtimeTLS.ServerKeyFile,
				},
			},
		},
		Data: &conf.Data{
			Database: &conf.Data_Database{
				Dsn: "postgres://platform_mihomo:password@127.0.0.1:5432/platform_mihomo?sslmode=disable",
			},
			Redis: &conf.Data_Redis{Addr: "127.0.0.1:6379"},
		},
		Metrics: &conf.Metrics{Addr: "127.0.0.1:0"},
		Upstream: &conf.Upstream{
			BaseUrl:        "https://mihomo-upstream.internal",
			TimeoutSeconds: 10,
		},
		Security: &conf.Security{
			CredentialEncryptionKeyringFile: mainTestEncryptionKeyringFile(t),
			ServiceTicketIssuer:             "paigram-account-center",
			ServiceTicketPublicKeyringFile:  mainTestPublicKeyringFile(t),
		},
	}
}

func TestNewTicketVerifierFromSecurityUsesConfiguredEd25519PublicKeyAndKID(t *testing.T) {
	verifier, err := newTicketVerifierFromSecurity(&conf.Security{
		ServiceTicketIssuer:            "paigram-account-center",
		ServiceTicketPublicKeyringFile: mainTestPublicKeyringFile(t),
	})
	if err != nil {
		t.Fatalf("newTicketVerifierFromSecurity() error = %v", err)
	}

	now := time.Now()
	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, jwt.MapClaims{
		"iss":            "paigram-account-center",
		"sub":            "user:usr-1",
		"owner_user_ref": "usr-1",
		"aud":            []string{"platform-mihomo-service"},
		"actor_type":     "user",
		"actor_id":       "user-paigram",
		"binding_ref":    "binding-101",
		"platform":       "mihomo",
		"iat":            now.Unix(),
		"nbf":            now.Add(-time.Second).Unix(),
		"exp":            now.Add(time.Minute).Unix(),
		"jti":            "main-test-ticket",
	})
	token.Header["kid"] = mainTestServiceTicketKeyID
	token.Header["typ"] = contractticket.TypeControl
	signed, err := token.SignedString(mainTestServiceTicketPrivateKey)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if _, err := verifier.Verify(signed, "platform-mihomo-service"); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestBuildProductionComponentsWiresHTTPUpstreamAndArtifactCleanup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer upstream.Close()
	bc := validMainTestBootstrap(t)
	bc.Upstream.BaseUrl = upstream.URL
	bc.Upstream.AllowInsecureHttp = true
	database, err := gorm.Open(sqlite.Open("file:composition-root?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()

	components, err := buildProductionComponents(bc, database, redisClient)
	require.NoError(t, err)
	require.NotNil(t, components.controlService)
	require.NotNil(t, components.runtimeService)
	require.NotNil(t, components.artifactCleanupServer)
	require.NotNil(t, components.credentialReencryptionServer)
	require.NotNil(t, components.metrics)
}

func mainTestPublicKeyPEM(t *testing.T) string {
	t.Helper()

	der, err := x509.MarshalPKIXPublicKey(mainTestServiceTicketPrivateKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func mainTestPublicKeyringFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service-ticket-keyring.json")
	raw, err := json.Marshal(contractticket.PublicKeyringFile{Keys: []contractticket.PublicKeyEntry{{
		KeyID: mainTestServiceTicketKeyID, PublicKeyPEM: mainTestPublicKeyPEM(t),
	}}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func mainTestEncryptionKeyringFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "encryption-keyring.json")
	raw, err := json.Marshal(internalcrypto.KeyringFile{
		ActiveKeyID: "main-test", Keys: []internalcrypto.KeyringEntry{{
			KeyID: "main-test", KeyBase64: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY",
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}
