package credentials

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/model"
)

// The signing key MUST be ≥ 32 bytes per the production caller's
// invariant; we use a single 32-byte test key everywhere.
var testSigningKey = []byte("0123456789abcdef0123456789abcdef")

// setupCredentialsTestDB opens an in-memory sqlite database and creates
// the service_credentials table directly via DDL. We do not call
// AutoMigrate on &model.ServiceCredential{} because GORM would walk its
// `Owner User` association and try to migrate the User model too —
// User's MySQL-specific column defaults (CURRENT_TIMESTAMP(3)) are not
// valid sqlite, so the migration would fail.
func setupCredentialsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS service_credentials`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE service_credentials (
		client_id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		secret_hash TEXT NOT NULL,
		audiences TEXT NOT NULL,
		scopes TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		owner_user_id INTEGER NOT NULL,
		description TEXT,
		last_used_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME
	)`).Error)
	return db
}

// seedOwnerID returns a deterministic non-zero user id to attach to
// credential rows. We do not insert into a users table because the
// User model is not migrated in this sqlite test setup (see
// setupCredentialsTestDB) — only the credential row's bigint column
// needs to hold a value.
func seedOwnerID(t *testing.T) uint64 {
	t.Helper()
	return 1
}

// newTestTokenService wires a TokenService + Service group around the
// supplied gorm.DB with a known signing key and 1-hour TTL.
func newTestTokenService(t *testing.T, db *gorm.DB) (*Service, *TokenService) {
	t.Helper()
	svc := NewService(db)
	tokenSvc, err := NewTokenService(svc, TokenServiceConfig{
		Issuer:                "account-center",
		AccessTokenTTLSeconds: 3600,
		SigningKey:            testSigningKey,
	})
	require.NoError(t, err)
	return svc, tokenSvc
}

// seedCredential creates a credential with the given client_id, audiences
// and scopes and returns the plaintext secret to the caller for use in
// VerifySecret / Issue tests.
func seedCredential(t *testing.T, svc *Service, ownerID uint64, clientID string, audiences, scopes []string) string {
	t.Helper()
	result, err := svc.Create(CreateInput{
		ClientID:    clientID,
		DisplayName: clientID,
		OwnerUserID: ownerID,
		Audiences:   audiences,
		Scopes:      scopes,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.ClientSecret)
	return result.ClientSecret
}

func TestTokenService_IssueAndValidateRoundTrip(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc, tokenSvc := newTestTokenService(t, db)

	secret := seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync", "account-center"},
		[]string{"binding.read", "binding.write"},
	)

	issued, err := tokenSvc.IssueClientCredentials(IssueClientCredentialsInput{
		ClientID:        "telegram-service",
		ClientSecret:    secret,
		Audience:        "mihomo.sync",
		RequestedScopes: []string{"binding.read"},
	})
	require.NoError(t, err)
	require.NotNil(t, issued)
	assert.Equal(t, "Bearer", issued.TokenType)
	assert.Equal(t, int64(3600), issued.ExpiresIn)
	assert.Equal(t, "binding.read", issued.Scope)

	claims, err := tokenSvc.ValidateAccessToken(issued.AccessToken, "mihomo.sync")
	require.NoError(t, err)
	assert.Equal(t, "telegram-service", claims.ClientID)
	assert.Equal(t, "binding.read", claims.Scope)
	assert.Contains(t, claims.Audience, "mihomo.sync")
}

func TestTokenService_IssueErrors(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc, tokenSvc := newTestTokenService(t, db)

	secret := seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync"},
		[]string{"binding.read", "binding.write"},
	)

	cases := []struct {
		name    string
		input   IssueClientCredentialsInput
		wantErr error
	}{
		{
			name:    "unknown client_id",
			input:   IssueClientCredentialsInput{ClientID: "nope", ClientSecret: secret, Audience: "mihomo.sync"},
			wantErr: ErrCredentialNotFound,
		},
		{
			name:    "wrong client_secret",
			input:   IssueClientCredentialsInput{ClientID: "telegram-service", ClientSecret: "wrong", Audience: "mihomo.sync"},
			wantErr: ErrInvalidClientSecret,
		},
		{
			name:    "audience not in credential allowed list",
			input:   IssueClientCredentialsInput{ClientID: "telegram-service", ClientSecret: secret, Audience: "other-audience"},
			wantErr: ErrInvalidAudience,
		},
		{
			name: "requested scope superset of granted",
			input: IssueClientCredentialsInput{
				ClientID: "telegram-service", ClientSecret: secret, Audience: "mihomo.sync",
				RequestedScopes: []string{"binding.read", "binding.delete"},
			},
			wantErr: ErrInsufficientScope,
		},
		{
			name:    "empty client_id",
			input:   IssueClientCredentialsInput{ClientID: "", ClientSecret: secret, Audience: "mihomo.sync"},
			wantErr: ErrEmptyClientID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tokenSvc.IssueClientCredentials(tc.input)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestTokenService_IssueWithoutRequestedScopeInheritsGranted(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc, tokenSvc := newTestTokenService(t, db)
	secret := seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync"},
		[]string{"binding.read", "binding.write"},
	)

	issued, err := tokenSvc.IssueClientCredentials(IssueClientCredentialsInput{
		ClientID:     "telegram-service",
		ClientSecret: secret,
		Audience:     "mihomo.sync",
		// RequestedScopes deliberately nil — RFC 6749 §3.3 says the
		// authorization server may grant the credential's full scope set.
	})
	require.NoError(t, err)
	// Order matches the credential's granted scope ordering.
	assert.Equal(t, "binding.read binding.write", issued.Scope)
}

func TestService_VerifySecret(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc := NewService(db)
	result, err := svc.Create(CreateInput{
		ClientID:    "telegram-service",
		DisplayName: "Telegram",
		OwnerUserID: owner,
		Audiences:   []string{"mihomo.sync"},
		Scopes:      []string{"binding.read"},
	})
	require.NoError(t, err)

	t.Run("happy path", func(t *testing.T) {
		row, err := svc.VerifySecret("telegram-service", result.ClientSecret)
		require.NoError(t, err)
		assert.Equal(t, "telegram-service", row.ClientID)
	})

	t.Run("invalid secret", func(t *testing.T) {
		_, err := svc.VerifySecret("telegram-service", "bogus")
		require.ErrorIs(t, err, ErrInvalidClientSecret)
	})

	t.Run("unknown client_id", func(t *testing.T) {
		_, err := svc.VerifySecret("does-not-exist", "anything")
		require.ErrorIs(t, err, ErrCredentialNotFound)
	})
}

func TestTokenService_ValidateRejectsExpiredToken(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc, tokenSvc := newTestTokenService(t, db)
	seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync"},
		[]string{"binding.read"},
	)

	// Hand-mint a JWT whose exp is in the past, using the same signing
	// key + issuer/audience the token service uses. This avoids time.Sleep
	// and keeps the test deterministic.
	expired := mintExpiredToken(t, "telegram-service", "mihomo.sync", "account-center")

	_, err := tokenSvc.ValidateAccessToken(expired, "mihomo.sync")
	require.Error(t, err)
	assert.ErrorIs(t, err, jwt.ErrTokenExpired)
}

func TestTokenService_ValidateRejectsDisabledCredential(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc, tokenSvc := newTestTokenService(t, db)
	secret := seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync"},
		[]string{"binding.read"},
	)

	issued, err := tokenSvc.IssueClientCredentials(IssueClientCredentialsInput{
		ClientID: "telegram-service", ClientSecret: secret, Audience: "mihomo.sync",
	})
	require.NoError(t, err)

	// Sanity check: token is valid before we disable the credential.
	_, err = tokenSvc.ValidateAccessToken(issued.AccessToken, "mihomo.sync")
	require.NoError(t, err)

	_, err = svc.SetStatus("telegram-service", model.ServiceCredentialStatusDisabled)
	require.NoError(t, err)

	// Path D §1.6: stateless tokens still re-check credential status
	// at Validate time. A disabled credential blocks the previously
	// issued token.
	_, err = tokenSvc.ValidateAccessToken(issued.AccessToken, "mihomo.sync")
	require.ErrorIs(t, err, ErrCredentialDisabled)
}

func TestTokenService_ValidateRejectsTamperedSignature(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc, tokenSvc := newTestTokenService(t, db)
	secret := seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync"},
		[]string{"binding.read"},
	)

	issued, err := tokenSvc.IssueClientCredentials(IssueClientCredentialsInput{
		ClientID: "telegram-service", ClientSecret: secret, Audience: "mihomo.sync",
	})
	require.NoError(t, err)

	tampered := flipLastChar(issued.AccessToken)
	require.NotEqual(t, issued.AccessToken, tampered)

	_, err = tokenSvc.ValidateAccessToken(tampered, "mihomo.sync")
	require.Error(t, err)
}

func TestTokenService_ValidateRejectsWrongIssuer(t *testing.T) {
	db := setupCredentialsTestDB(t)
	owner := seedOwnerID(t)
	svc := NewService(db)
	secret := seedCredential(t, svc, owner, "telegram-service",
		[]string{"mihomo.sync"},
		[]string{"binding.read"},
	)

	issuerA, err := NewTokenService(svc, TokenServiceConfig{
		Issuer:                "foo",
		AccessTokenTTLSeconds: 3600,
		SigningKey:            testSigningKey,
	})
	require.NoError(t, err)
	issuerB, err := NewTokenService(svc, TokenServiceConfig{
		Issuer:                "bar",
		AccessTokenTTLSeconds: 3600,
		SigningKey:            testSigningKey,
	})
	require.NoError(t, err)

	issued, err := issuerA.IssueClientCredentials(IssueClientCredentialsInput{
		ClientID: "telegram-service", ClientSecret: secret, Audience: "mihomo.sync",
	})
	require.NoError(t, err)

	_, err = issuerB.ValidateAccessToken(issued.AccessToken, "mihomo.sync")
	require.Error(t, err, "token signed by issuer 'foo' must not validate under issuer 'bar'")
}

// mintExpiredToken builds a JWT whose `exp` is one hour in the past,
// signed with the same HS256 key the test TokenService uses.
func mintExpiredToken(t *testing.T, clientID, audience, issuer string) string {
	t.Helper()
	now := time.Now().UTC().Add(-2 * time.Hour)
	claims := AccessClaims{
		ClientID: clientID,
		Scope:    "binding.read",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   clientID,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(testSigningKey)
	require.NoError(t, err)
	return signed
}

// flipLastChar swaps the final character of the JWT with a different
// base64url character so the signature segment no longer verifies.
func flipLastChar(token string) string {
	if token == "" {
		return token
	}
	last := token[len(token)-1]
	replacement := byte('A')
	if last == 'A' {
		replacement = 'B'
	}
	return token[:len(token)-1] + string(replacement)
}
