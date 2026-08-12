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
