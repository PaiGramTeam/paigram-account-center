package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

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

func TestSetDefaultsIncludesServiceTicketSettings(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	require.Equal(t, 300, v.GetInt("auth.service_ticket_ttl"))
	require.Equal(t, "paigram-account-center", v.GetString("auth.service_ticket_issuer"))
	require.Empty(t, v.GetString("auth.service_ticket_key_id"))
	require.Empty(t, v.GetString("auth.service_ticket_private_key_pem"))
	require.Empty(t, v.GetString("auth.oauth_signing_key"))
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
		ServiceTicketTTLSeconds:    300,
		ServiceTicketIssuer:        "paigram-account-center",
		ServiceTicketKeyID:         "account-center-2026-08",
		ServiceTicketPrivateKeyPEM: "not-a-private-key",
		OAuthSigningKey:            "this-is-a-valid-32-byte-oauth-key!",
	}})
	require.Error(t, err)
}

func TestValidateServiceTicketConfigRejectsEmptyOAuthSigningKey(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds:    300,
		ServiceTicketIssuer:        "paigram-account-center",
		ServiceTicketKeyID:         "account-center-2026-08",
		ServiceTicketPrivateKeyPEM: testServiceTicketPrivateKeyPEM(t),
	}})
	require.Error(t, err)
}

func TestValidateServiceTicketConfigAcceptsSeparatedKeys(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds:    300,
		ServiceTicketIssuer:        "paigram-account-center",
		ServiceTicketKeyID:         "account-center-2026-08",
		ServiceTicketPrivateKeyPEM: testServiceTicketPrivateKeyPEM(t),
		OAuthSigningKey:            "this-is-a-valid-32-byte-oauth-key!",
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
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
}
