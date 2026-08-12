package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/conf"
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

	t.Run("rejects missing grpc address", func(t *testing.T) {
		bc := &conf.Bootstrap{
			Server: &conf.Server{
				Grpc: &conf.Server_GRPC{
					Network:        "tcp",
					TimeoutSeconds: 5,
				},
			},
			Security: &conf.Security{
				CredentialEncryptionKey:   "0123456789abcdef0123456789abcdef",
				ServiceTicketIssuer:       "paigram-account-center",
				ServiceTicketKeyId:        mainTestServiceTicketKeyID,
				ServiceTicketPublicKeyPem: mainTestPublicKeyPEM(t),
			},
		}

		if err := validateBootstrap(bc); err == nil {
			t.Fatal("validateBootstrap() error = nil, want non-nil")
		}
	})

	t.Run("rejects missing service ticket key id", func(t *testing.T) {
		bc := &conf.Bootstrap{
			Server: &conf.Server{
				Grpc: &conf.Server_GRPC{
					Network:        "tcp",
					Addr:           "127.0.0.1:9000",
					TimeoutSeconds: 5,
				},
			},
			Security: &conf.Security{
				CredentialEncryptionKey:   "0123456789abcdef0123456789abcdef",
				ServiceTicketIssuer:       "paigram-account-center",
				ServiceTicketPublicKeyPem: mainTestPublicKeyPEM(t),
			},
		}

		if err := validateBootstrap(bc); err == nil {
			t.Fatal("validateBootstrap() error = nil, want non-nil")
		}
	})

	t.Run("rejects missing service ticket public key pem", func(t *testing.T) {
		bc := &conf.Bootstrap{
			Server: &conf.Server{
				Grpc: &conf.Server_GRPC{
					Network:        "tcp",
					Addr:           "127.0.0.1:9000",
					TimeoutSeconds: 5,
				},
			},
			Security: &conf.Security{
				CredentialEncryptionKey: "0123456789abcdef0123456789abcdef",
				ServiceTicketIssuer:     "paigram-account-center",
				ServiceTicketKeyId:      mainTestServiceTicketKeyID,
			},
		}

		if err := validateBootstrap(bc); err == nil {
			t.Fatal("validateBootstrap() error = nil, want non-nil")
		}
	})
}

func validMainTestBootstrap(t *testing.T) *conf.Bootstrap {
	t.Helper()
	return &conf.Bootstrap{
		Server: &conf.Server{
			Grpc: &conf.Server_GRPC{
				Network:        "tcp",
				Addr:           "127.0.0.1:9000",
				TimeoutSeconds: 5,
			},
		},
		Data: &conf.Data{
			Database: &conf.Data_Database{
				Dsn: "postgres://platform_mihomo:password@127.0.0.1:5432/platform_mihomo?sslmode=disable",
			},
		},
		Upstream: &conf.Upstream{
			BaseUrl:        "https://mihomo-upstream.internal",
			TimeoutSeconds: 10,
		},
		Security: &conf.Security{
			CredentialEncryptionKey:   "0123456789abcdef0123456789abcdef",
			ServiceTicketIssuer:       "paigram-account-center",
			ServiceTicketKeyId:        mainTestServiceTicketKeyID,
			ServiceTicketPublicKeyPem: mainTestPublicKeyPEM(t),
		},
	}
}

func TestNewTicketVerifierFromSecurityUsesConfiguredEd25519PublicKeyAndKID(t *testing.T) {
	verifier, err := newTicketVerifierFromSecurity(&conf.Security{
		ServiceTicketIssuer:       "paigram-account-center",
		ServiceTicketKeyId:        mainTestServiceTicketKeyID,
		ServiceTicketPublicKeyPem: mainTestPublicKeyPEM(t),
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

func mainTestPublicKeyPEM(t *testing.T) string {
	t.Helper()

	der, err := x509.MarshalPKIXPublicKey(mainTestServiceTicketPrivateKey.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error = %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}
