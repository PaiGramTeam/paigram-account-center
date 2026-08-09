package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"platform-mihomo-service/internal/conf"
)

const mainTestServiceTicketKeyID = "main-test-key"

var mainTestServiceTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))

func TestValidateBootstrap(t *testing.T) {
	t.Run("accepts valid bootstrap", func(t *testing.T) {
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
				ServiceTicketKeyId:        mainTestServiceTicketKeyID,
				ServiceTicketPublicKeyPem: mainTestPublicKeyPEM(t),
			},
		}

		if err := validateBootstrap(bc); err != nil {
			t.Fatalf("validateBootstrap() error = %v", err)
		}
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

func TestNewTicketVerifierFromSecurityUsesConfiguredEd25519PublicKeyAndKID(t *testing.T) {
	verifier, err := newTicketVerifierFromSecurity(&conf.Security{
		ServiceTicketIssuer:       "paigram-account-center",
		ServiceTicketKeyId:        mainTestServiceTicketKeyID,
		ServiceTicketPublicKeyPem: mainTestPublicKeyPEM(t),
	})
	if err != nil {
		t.Fatalf("newTicketVerifierFromSecurity() error = %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"iss":           "paigram-account-center",
		"aud":           []string{"platform-mihomo-service"},
		"actor_type":    "bot",
		"actor_id":      "bot-paigram",
		"owner_user_id": float64(1),
		"binding_id":    float64(101),
		"platform":      "mihomo",
		"exp":           time.Now().Add(time.Minute).Unix(),
	})
	token.Header["kid"] = mainTestServiceTicketKeyID
	token.Header["typ"] = "service_ticket"
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
