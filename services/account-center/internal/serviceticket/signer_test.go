package serviceticket

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestSignerIssuesFullySpecifiedDelegationTicket(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})

	signer, err := NewSigner(Config{
		Issuer:        "paigram-account-center",
		KeyID:         "account-center-2026-08",
		TTL:           2 * time.Minute,
		PrivateKeyPEM: string(privatePEM),
	})
	require.NoError(t, err)

	raw, expiresAt, err := signer.Issue(TypeDelegation, "consumer:paigram", "platform-mihomo-service", Claims{
		ActorType:      "consumer",
		ActorID:        "paigram",
		Consumer:       "paigram",
		OwnerUserRef:   "usr-42",
		BindingRef:     "binding-101",
		Platform:       "mihomo",
		GrantVersion:   7,
		AllowedActions: []string{"mihomo.profile.read"},
	})
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().UTC().Add(2*time.Minute), expiresAt, 2*time.Second)

	parsedClaims := &Claims{}
	parsed, err := jwt.ParseWithClaims(raw, parsedClaims, func(token *jwt.Token) (any, error) {
		require.Equal(t, contractticket.AlgorithmEd25519, token.Method.Alg())
		require.Equal(t, "account-center-2026-08", token.Header["kid"])
		require.Equal(t, TypeDelegation, token.Header["typ"])
		return publicKey, nil
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}), jwt.WithIssuer("paigram-account-center"), jwt.WithAudience("platform-mihomo-service"), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	require.Equal(t, "consumer:paigram", parsedClaims.Subject)
	require.NotNil(t, parsedClaims.NotBefore)
	require.NotEmpty(t, parsedClaims.ID)
	require.Equal(t, []string{"mihomo.profile.read"}, parsedClaims.AllowedActions)
}
