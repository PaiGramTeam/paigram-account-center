package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestSetDefaultsIncludesSentry(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	require.False(t, v.GetBool("sentry.enabled"))
	require.Empty(t, v.GetString("sentry.dsn"))
	require.Equal(t, "development", v.GetString("sentry.environment"))
	require.Empty(t, v.GetString("sentry.release"))
	require.False(t, v.GetBool("sentry.debug"))
	require.True(t, v.GetBool("sentry.attach_stacktrace"))
	require.Equal(t, 1.0, v.GetFloat64("sentry.sample_rate"))
	require.Equal(t, 0.0, v.GetFloat64("sentry.traces_sample_rate"))
	require.Equal(t, 2, v.GetInt("sentry.flush_timeout"))
	require.False(t, v.GetBool("auth.session_cookie_secure"))
}

func TestTrustedProxiesCanBeConfiguredFromEnvironment(t *testing.T) {
	t.Setenv("PAI_APP_TRUSTED_PROXIES", "10.77.20.10")
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("PAI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	loaded := &Config{}
	require.NoError(t, v.Unmarshal(loaded))
	require.Equal(t, []string{"10.77.20.10"}, loaded.App.TrustedProxies)
}

func TestSecretFilePathsCanBeConfiguredFromEnvironment(t *testing.T) {
	paths := map[string]string{
		"PAI_DATABASE_DSN_FILE":                    "/run/secrets/account-database-dsn",
		"PAI_REDIS_PASSWORD_FILE":                  "/run/secrets/account-redis-password",
		"PAI_AUTH_SERVICE_TICKET_SIGNING_KEY_FILE": "/run/secrets/account-ticket-key",
		"PAI_AUTH_OAUTH_SIGNING_KEY_FILE":          "/run/secrets/account-oauth-key",
		"PAI_SECURITY_ENCRYPTION_KEY_FILE":         "/run/secrets/account-encryption-key",
		"PAI_PLATFORM_CONTROL_ROOT_CA_FILE":        "/run/secrets/platform-control-ca",
		"PAI_PLATFORM_CONTROL_CERTIFICATE_FILE":    "/run/secrets/account-control-cert",
		"PAI_PLATFORM_CONTROL_PRIVATE_KEY_FILE":    "/run/secrets/account-control-key",
		"PAI_GRPC_CERTIFICATE_FILE":                "/run/secrets/account-grpc-cert",
		"PAI_GRPC_PRIVATE_KEY_FILE":                "/run/secrets/account-grpc-key",
	}
	for name, value := range paths {
		t.Setenv(name, value)
	}
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("PAI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	loaded := &Config{}
	require.NoError(t, v.Unmarshal(loaded))
	require.Equal(t, paths["PAI_DATABASE_DSN_FILE"], loaded.Database.DSNFile)
	require.Equal(t, paths["PAI_REDIS_PASSWORD_FILE"], loaded.Redis.PasswordFile)
	require.Equal(t, paths["PAI_AUTH_SERVICE_TICKET_SIGNING_KEY_FILE"], loaded.Auth.ServiceTicketSigningKeyFile)
	require.Equal(t, paths["PAI_AUTH_OAUTH_SIGNING_KEY_FILE"], loaded.Auth.OAuthSigningKeyFile)
	require.Equal(t, paths["PAI_SECURITY_ENCRYPTION_KEY_FILE"], loaded.Security.EncryptionKeyFile)
	require.Equal(t, paths["PAI_PLATFORM_CONTROL_ROOT_CA_FILE"], loaded.PlatformControl.RootCAFile)
	require.Equal(t, paths["PAI_PLATFORM_CONTROL_CERTIFICATE_FILE"], loaded.PlatformControl.CertificateFile)
	require.Equal(t, paths["PAI_PLATFORM_CONTROL_PRIVATE_KEY_FILE"], loaded.PlatformControl.PrivateKeyFile)
	require.Equal(t, paths["PAI_GRPC_CERTIFICATE_FILE"], loaded.GRPC.CertificateFile)
	require.Equal(t, paths["PAI_GRPC_PRIVATE_KEY_FILE"], loaded.GRPC.PrivateKeyFile)
}

func TestSetDefaultsIncludesServiceTicketSettings(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	require.Equal(t, 300, v.GetInt("auth.service_ticket_ttl"))
	require.Equal(t, "paigram-account-center", v.GetString("auth.service_ticket_issuer"))
	require.Empty(t, v.GetString("auth.service_ticket_signing_key_file"))
	require.Empty(t, v.GetString("auth.oauth_signing_key"))
}

func TestValidatePlatformControlConfigRequiresCompleteMutualTLSIdentity(t *testing.T) {
	bundle := tlstest.New(t, "control.internal")
	valid := PlatformControlConfig{
		RootCAFile:      bundle.CAFile,
		CertificateFile: bundle.ClientCertFile,
		PrivateKeyFile:  bundle.ClientKeyFile,
		ServerName:      bundle.ServerName,
		DialTimeout:     5 * time.Second,
	}
	require.NoError(t, validatePlatformControlConfig(&Config{PlatformControl: valid}))

	missingCertificate := valid
	missingCertificate.CertificateFile = ""
	require.Error(t, validatePlatformControlConfig(&Config{PlatformControl: missingCertificate}))

	missingServerName := valid
	missingServerName.ServerName = ""
	require.Error(t, validatePlatformControlConfig(&Config{PlatformControl: missingServerName}))
}

func TestValidateGRPCServerConfigRequiresTLSWhenEnabled(t *testing.T) {
	require.Error(t, validateGRPCServerConfig(&Config{GRPC: GRPCConfig{Enabled: true}}))
	bundle := tlstest.New(t, "account.internal")
	require.NoError(t, validateGRPCServerConfig(&Config{GRPC: GRPCConfig{
		Enabled: true, CertificateFile: bundle.ServerCertFile, PrivateKeyFile: bundle.ServerKeyFile,
	}}))
}

func TestResolveSecretFilesOverridesInlineValues(t *testing.T) {
	directory := t.TempDir()
	writeSecret := func(name, value string) string {
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, []byte(value+"\n"), 0o600))
		return path
	}
	loaded := &Config{
		Database: DatabaseConfig{DSN: "inline", DSNFile: writeSecret("database-dsn", "postgres://secure")},
		Redis:    RedisConfig{Password: "inline", PasswordFile: writeSecret("redis-password", "redis-secret")},
		Auth:     AuthConfig{OAuthSigningKey: "inline", OAuthSigningKeyFile: writeSecret("oauth-key", "oauth-secret")},
		Security: SecurityConfig{EncryptionKey: "inline", EncryptionKeyFile: writeSecret("encryption-key", "encryption-secret")},
	}
	require.NoError(t, resolveSecretFiles(loaded))
	require.Equal(t, "postgres://secure", loaded.Database.DSN)
	require.Equal(t, "redis-secret", loaded.Redis.Password)
	require.Equal(t, "oauth-secret", loaded.Auth.OAuthSigningKey)
	require.Equal(t, "encryption-secret", loaded.Security.EncryptionKey)
}

// TestSetDefaultsIncludesOAuthSettings covers the Path D §3.2 +
// §3.8 additions: a default OAuth issuer ("account-center" — same string
// the gRPC interceptor's machineTokenAudience constant expects) and a
// 1-hour access-token TTL (Path D §1.6: revocation = short TTL +
// credential disable).
func TestSetDefaultsIncludesOAuthSettings(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	require.Equal(t, "account-center", v.GetString("auth.oauth_issuer"))
	require.Equal(t, 3600, v.GetInt("auth.oauth_access_token_ttl_seconds"))
}

func TestValidateServiceTicketConfigRejectsInvalidPrivateKey(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds:     300,
		ServiceTicketIssuer:         "paigram-account-center",
		ServiceTicketSigningKeyFile: testServiceTicketSigningKeyFile(t, "not-a-private-key"),
		OAuthSigningKey:             "this-is-a-valid-32-byte-oauth-key!",
	}})
	require.Error(t, err)
}

func TestValidateServiceTicketConfigRejectsEmptyOAuthSigningKey(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds:     300,
		ServiceTicketIssuer:         "paigram-account-center",
		ServiceTicketSigningKeyFile: testServiceTicketSigningKeyFile(t, testServiceTicketPrivateKeyPEM(t)),
	}})
	require.Error(t, err)
}

func TestValidateServiceTicketConfigAcceptsSeparatedKeys(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds:     300,
		ServiceTicketIssuer:         "paigram-account-center",
		ServiceTicketSigningKeyFile: testServiceTicketSigningKeyFile(t, testServiceTicketPrivateKeyPEM(t)),
		OAuthSigningKey:             "this-is-a-valid-32-byte-oauth-key!",
	}})
	require.NoError(t, err)
}

func TestValidateEmailDeliveryConfigFailsClosedForReleaseVerification(t *testing.T) {
	err := validateEmailDeliveryConfig(&Config{
		App:      AppConfig{Mode: "release"},
		Auth:     AuthConfig{RequireEmailVerificationLogin: true},
		Frontend: FrontendConfig{BaseURL: "https://account.example.com"},
		Email:    EmailConfig{Enabled: false},
	})
	require.Error(t, err)

	err = validateEmailDeliveryConfig(&Config{
		App:      AppConfig{Mode: "release"},
		Auth:     AuthConfig{RequireEmailVerificationLogin: true},
		Frontend: FrontendConfig{BaseURL: "https://account.example.com"},
		Email:    EmailConfig{Enabled: true},
	})
	require.NoError(t, err)
}

func TestValidateEmailDeliveryConfigAllowsExplicitPreproductionOptOut(t *testing.T) {
	err := validateEmailDeliveryConfig(&Config{
		App:  AppConfig{Mode: "release"},
		Auth: AuthConfig{RequireEmailVerificationLogin: false},
	})
	require.NoError(t, err)
}

func TestValidateBrowserSessionConfigFailsClosedInRelease(t *testing.T) {
	err := validateBrowserSessionConfig(&Config{
		App:      AppConfig{Mode: "release"},
		Auth:     AuthConfig{SessionCookieSecure: false},
		Frontend: FrontendConfig{BaseURL: "https://account.example.com"},
	})
	require.Error(t, err)

	err = validateBrowserSessionConfig(&Config{
		App:      AppConfig{Mode: "release"},
		Auth:     AuthConfig{SessionCookieSecure: true},
		Frontend: FrontendConfig{BaseURL: "http://account.example.com"},
	})
	require.Error(t, err)

	err = validateBrowserSessionConfig(&Config{
		App:      AppConfig{Mode: "release"},
		Auth:     AuthConfig{SessionCookieSecure: true},
		Frontend: FrontendConfig{BaseURL: "https://account.example.com"},
	})
	require.NoError(t, err)
}

func testServiceTicketPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	privateKeyPEM, _, err := contractticket.GenerateKeyPairPEM()
	require.NoError(t, err)
	return privateKeyPEM
}

func testServiceTicketSigningKeyFile(t *testing.T, privateKeyPEM string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service-ticket-signing-key.json")
	raw, err := json.Marshal(contractticket.SigningKeyFile{KeyID: "account-center-2026-08", PrivateKeyPEM: privateKeyPEM})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}
