package usecase

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"platform-mihomo-service/internal/biz"
	internalcrypto "platform-mihomo-service/internal/crypto"
	"platform-mihomo-service/internal/data"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

const statusUsecaseTicketKeyID = "status-usecase-test-key"

var statusUsecaseTicketPrivateKey = ed25519.NewKeyFromSeed([]byte("0123456789abcdef0123456789abcdef"))

func TestVerifyServiceTicketAcceptsExpectedClaims(t *testing.T) {
	verifier := data.NewStaticKeyTicketVerifier("paigram-account-center", statusUsecaseTicketKeyID, statusUsecaseTicketPrivateKey.Public().(ed25519.PublicKey))
	signed := signedStatusUsecaseTicket(t, jwt.MapClaims{
		"iss":                  "paigram-account-center",
		"aud":                  []string{"platform-mihomo-service"},
		"actor_type":           "user",
		"actor_id":             "user-paigram",
		"binding_ref":          "binding-101",
		"bot_id":               "bot-paigram",
		"platform":             "mihomo",
		"platform_service_key": "platform-mihomo-service",
		"account_key":          "binding_101_10001",
		"scopes":               []string{"mihomo.status.read"},
		"exp":                  time.Now().Add(time.Minute).Unix(),
	})

	claims, err := verifier.Verify(signed, "platform-mihomo-service")
	require.NoError(t, err)
	require.Equal(t, "bot-paigram", claims.BotID)
	require.Equal(t, "user", claims.ActorType)
	require.Equal(t, "user-paigram", claims.ActorID)
	require.Equal(t, "binding-101", claims.BindingRef)
	require.Equal(t, "mihomo", claims.Platform)
	require.Equal(t, "platform-mihomo-service", claims.PlatformServiceKey)
	require.Equal(t, "binding_101_10001", claims.AccountKey)
	require.Equal(t, []string{"mihomo.status.read"}, claims.Scopes)
}

func TestVerifyServiceTicketRejectsMissingRequiredClaims(t *testing.T) {
	verifier := data.NewStaticKeyTicketVerifier("paigram-account-center", statusUsecaseTicketKeyID, statusUsecaseTicketPrivateKey.Public().(ed25519.PublicKey))
	signed := signedStatusUsecaseTicket(t, jwt.MapClaims{
		"iss": "paigram-account-center",
		"aud": []string{"platform-mihomo-service"},
		"exp": time.Now().Add(time.Minute).Unix(),
	})

	_, err := verifier.Verify(signed, "platform-mihomo-service")
	require.Error(t, err)
}

func signedStatusUsecaseTicket(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	now := time.Now()
	claims["sub"] = "user:usr-1"
	claims["owner_user_ref"] = "usr-1"
	claims["iat"] = now.Unix()
	claims["nbf"] = now.Add(-time.Second).Unix()
	claims["jti"] = "status-usecase-test-ticket"
	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, claims)
	token.Header["kid"] = statusUsecaseTicketKeyID
	token.Header["typ"] = contractticket.TypeControl
	signed, err := token.SignedString(statusUsecaseTicketPrivateKey)
	require.NoError(t, err)
	return signed
}

func TestRefreshCredentialDoesNotMarkRefreshedOnValidationFailure(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	encryptedBlob, err := internalcrypto.EncryptString(testEncryptionKey, `{"account_id":"10001","cookie_token":"abc"}`)
	require.NoError(t, err)
	credentialRepo.byAccountKey["hoyo_10001"] = &biz.Credential{
		AccountKey:        "hoyo_10001",
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    encryptedBlob,
		CredentialVersion: "v1",
		Status:            "active",
	}
	uc := NewStatusUsecase(credentialRepo, failingStatusClient{err: &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorExpiredCredential}}, testEncryptionKey)

	resp, err := uc.RefreshCredential(context.Background(), "hoyo_10001")
	require.NoError(t, err)
	require.Equal(t, CredentialStatusExpired, resp.Status)
	require.Nil(t, resp.RefreshedAt)
	require.Nil(t, credentialRepo.byAccountKey["hoyo_10001"].LastRefreshedAt)
}

func TestRefreshCredentialRevalidatesWithoutReplacingCredentialBlob(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	encryptedBlob, err := internalcrypto.EncryptString(testEncryptionKey, `{"account_id":"10001","cookie_token":"abc"}`)
	require.NoError(t, err)
	require.NoError(t, credentialRepo.Save(context.Background(), &biz.Credential{
		BindingRef:        "binding-101",
		AccountKey:        "hoyo_10001",
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    encryptedBlob,
		CredentialVersion: "v1",
		Status:            "active",
	}))
	uc := NewStatusUsecase(credentialRepo, successfulStatusClient{}, testEncryptionKey)

	resp, err := uc.RefreshCredential(context.Background(), "hoyo_10001")
	require.NoError(t, err)
	require.Equal(t, CredentialStatusActive, resp.Status)

	stored := credentialRepo.byAccountKey["hoyo_10001"]
	require.Equal(t, encryptedBlob, stored.CredentialBlob)
	require.NotNil(t, stored.LastValidatedAt)
	require.NotNil(t, stored.LastRefreshedAt)
	require.Equal(t, stored.LastRefreshedAt, resp.RefreshedAt)
}

func TestValidateCredentialMapsChallengeRequiredStatus(t *testing.T) {
	credentialRepo := newMemoryCredentialRepo()
	encryptedBlob, err := internalcrypto.EncryptString(testEncryptionKey, `{"account_id":"10001","cookie_token":"abc"}`)
	require.NoError(t, err)
	credentialRepo.byAccountKey["hoyo_10001"] = &biz.Credential{
		AccountKey:        "hoyo_10001",
		Platform:          "mihomo",
		AccountID:         "10001",
		Region:            "cn_gf01",
		CredentialBlob:    encryptedBlob,
		CredentialVersion: "v1",
		Status:            "active",
	}
	uc := NewStatusUsecase(credentialRepo, failingStatusClient{err: &platformmihomo.UpstreamError{Kind: platformmihomo.ErrorChallengeRequired}}, testEncryptionKey)

	resp, err := uc.ValidateCredential(context.Background(), "hoyo_10001")
	require.NoError(t, err)
	require.Equal(t, CredentialStatusChallengeRequired, resp.Status)
	require.Equal(t, "CHALLENGE_REQUIRED", resp.ErrorCode)
}

type failingStatusClient struct {
	err error
}

type successfulStatusClient struct{}

func (successfulStatusClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "10001", "cn_gf01", []platformmihomo.DiscoveredProfile{{
		GameBiz:  "hk4e_cn",
		Region:   "cn_gf01",
		PlayerID: "1008611",
		Nickname: "Traveler",
		Level:    60,
	}}, nil
}

func (successfulStatusClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "", 0, errors.New("not implemented")
}

func (c failingStatusClient) ValidateAndDiscover(_ context.Context, _ string, _ string) (string, string, []platformmihomo.DiscoveredProfile, error) {
	return "", "", nil, c.err
}

func (c failingStatusClient) IssueAuthKey(_ context.Context, _ string, _ string) (string, int64, error) {
	return "", 0, errors.New("not implemented")
}
