package serviceticket

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const maximumTTL = 5 * time.Minute

var ErrInvalidIssuerConfig = errors.New("invalid service ticket issuer configuration")

type IssuerConfig struct {
	Issuer        string
	KeyID         string
	TTL           time.Duration
	PrivateKeyPEM string
}

type Claims struct {
	ActorType            string   `json:"actor_type"`
	ActorID              string   `json:"actor_id"`
	Consumer             string   `json:"consumer,omitempty"`
	ConsumerPrincipal    string   `json:"consumer_principal,omitempty"`
	ClientID             string   `json:"client_id,omitempty"`
	OwnerUserRef         string   `json:"owner_user_ref"`
	EntryIdentityRef     string   `json:"entry_identity_ref,omitempty"`
	BotID                string   `json:"bot_id,omitempty"`
	Platform             string   `json:"platform"`
	PlatformServiceKey   string   `json:"platform_service_key,omitempty"`
	BindingRef           string   `json:"binding_ref"`
	ProfileRef           string   `json:"profile_ref,omitempty"`
	GrantVersion         uint64   `json:"grant_version,omitempty"`
	OwnerEpoch           uint64   `json:"owner_epoch,omitempty"`
	ConsumerEpoch        uint64   `json:"consumer_epoch,omitempty"`
	EntryEpoch           uint64   `json:"entry_epoch,omitempty"`
	CredentialGeneration uint64   `json:"credential_generation,omitempty"`
	OperationID          string   `json:"operation_id,omitempty"`
	AccountKey           string   `json:"account_key,omitempty"`
	Scopes               []string `json:"scopes,omitempty"`
	AllowedActions       []string `json:"allowed_actions,omitempty"`
	jwt.RegisteredClaims
}

type Issuer struct {
	issuer     string
	keyID      string
	ttl        time.Duration
	privateKey ed25519.PrivateKey
}

func NewIssuer(config IssuerConfig) (*Issuer, error) {
	if config.Issuer == "" || config.KeyID == "" || config.TTL <= 0 || config.TTL > maximumTTL {
		return nil, ErrInvalidIssuerConfig
	}
	privateKey, err := ParsePrivateKeyPEM(config.PrivateKeyPEM)
	if err != nil {
		return nil, errors.Join(ErrInvalidIssuerConfig, err)
	}
	return &Issuer{issuer: config.Issuer, keyID: config.KeyID, ttl: config.TTL, privateKey: privateKey}, nil
}

func (s *Issuer) Issue(ticketType, subject, audience string, claims Claims) (string, time.Time, error) {
	if !validSubject(ticketType, subject) || audience == "" {
		return "", time.Time{}, ErrInvalidIssuerConfig
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)
	tokenID, err := newTokenID()
	if err != nil {
		return "", time.Time{}, err
	}
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer: s.issuer, Subject: subject, Audience: jwt.ClaimStrings{audience},
		ExpiresAt: jwt.NewNumericDate(expiresAt), NotBefore: jwt.NewNumericDate(now),
		IssuedAt: jwt.NewNumericDate(now), ID: tokenID,
	}
	token := jwt.NewWithClaims(SigningMethodEd25519, claims)
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
