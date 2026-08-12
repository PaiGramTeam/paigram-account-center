package serviceticket

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"
)

const (
	TypeControl    = contractticket.TypeControl
	TypeDelegation = contractticket.TypeDelegation
	maximumTTL     = 5 * time.Minute
)

var ErrInvalidConfig = errors.New("invalid service ticket configuration")

type Config struct {
	Issuer        string
	KeyID         string
	TTL           time.Duration
	PrivateKeyPEM string
}

type Claims struct {
	ActorType          string   `json:"actor_type"`
	ActorID            string   `json:"actor_id"`
	Consumer           string   `json:"consumer,omitempty"`
	ClientID           string   `json:"client_id,omitempty"`
	OwnerUserID        uint64   `json:"owner_user_id"`
	BotID              string   `json:"bot_id,omitempty"`
	UserID             uint64   `json:"user_id,omitempty"`
	Platform           string   `json:"platform"`
	PlatformServiceKey string   `json:"platform_service_key,omitempty"`
	BindingID          uint64   `json:"binding_id"`
	ProfileID          uint64   `json:"profile_id,omitempty"`
	GrantVersion       uint64   `json:"grant_version,omitempty"`
	PlatformAccountID  string   `json:"platform_account_id,omitempty"`
	Scopes             []string `json:"scopes,omitempty"`
	AllowedActions     []string `json:"allowed_actions,omitempty"`
	jwt.RegisteredClaims
}

type Signer struct {
	issuer     string
	keyID      string
	ttl        time.Duration
	privateKey ed25519.PrivateKey
}

func NewSigner(config Config) (*Signer, error) {
	if config.Issuer == "" || config.KeyID == "" || config.TTL <= 0 || config.TTL > maximumTTL {
		return nil, ErrInvalidConfig
	}
	privateKey, err := contractticket.ParsePrivateKeyPEM(config.PrivateKeyPEM)
	if err != nil {
		return nil, errors.Join(ErrInvalidConfig, err)
	}

	return &Signer{
		issuer:     config.Issuer,
		keyID:      config.KeyID,
		ttl:        config.TTL,
		privateKey: privateKey,
	}, nil
}

func (s *Signer) Issue(ticketType, subject, audience string, claims Claims) (string, time.Time, error) {
	if !validSubject(ticketType, subject) || audience == "" {
		return "", time.Time{}, ErrInvalidConfig
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)
	tokenID, err := newTokenID()
	if err != nil {
		return "", time.Time{}, err
	}
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    s.issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        tokenID,
	}

	token := jwt.NewWithClaims(contractticket.SigningMethodEd25519, claims)
	token.Header["kid"] = s.keyID
	token.Header["typ"] = ticketType
	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

func validSubject(ticketType, subject string) bool {
	switch ticketType {
	case TypeControl:
		return strings.HasPrefix(subject, "user:") || subject == "system:account-center"
	case TypeDelegation:
		return strings.HasPrefix(subject, "consumer:")
	default:
		return false
	}
}

func newTokenID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
