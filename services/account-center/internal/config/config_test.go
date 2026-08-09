package config

import (
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
}

func TestSetDefaultsIncludesServiceTicketSettings(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	require.Equal(t, 300, v.GetInt("auth.service_ticket_ttl"))
	require.Equal(t, "paigram-account-center", v.GetString("auth.service_ticket_issuer"))
	require.Empty(t, v.GetString("auth.service_ticket_signing_key"))
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

func TestValidateServiceTicketConfigRejectsShortSigningKey(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "paigram-account-center",
		ServiceTicketSigningKey: "short-key",
	}})
	require.Error(t, err)
}

// TestValidateServiceTicketConfigRejectsEmptySigningKey covers the Path D
// hardening: account-center now serves OAuth tokens whose HS256 signing
// key is the same SHARED_TICKET_KEY (§1.4). Empty is no longer a tolerated
// "off" state — config load must fail closed.
func TestValidateServiceTicketConfigRejectsEmptySigningKey(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "paigram-account-center",
		ServiceTicketSigningKey: "",
	}})
	require.Error(t, err)
}

func TestValidateServiceTicketConfigAcceptsValidSigningKey(t *testing.T) {
	err := validateServiceTicketConfig(&Config{Auth: AuthConfig{
		ServiceTicketTTLSeconds: 300,
		ServiceTicketIssuer:     "paigram-account-center",
		ServiceTicketSigningKey: "this-is-a-valid-32-byte-signing-key!!",
	}})
	require.NoError(t, err)
}
