package botaccess

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"paigram/internal/config"
	"paigram/internal/model"
)

// ServiceTicketClaims carries the user-scoped, per-Dispatch ticket that
// botaccess issues for telegram-service → genshin / mihomo handoff. The
// shape is unchanged from f27c395; only the signing algorithm reverts
// from EdDSA back to the spec-§9.3-mandated HS256 (Path D §3.5 + Q1).
type ServiceTicketClaims struct {
	Type               string   `json:"typ,omitempty"`
	ActorType          string   `json:"actor_type,omitempty"`
	ActorID            string   `json:"actor_id,omitempty"`
	Consumer           string   `json:"consumer,omitempty"`
	ClientID           string   `json:"client_id,omitempty"`
	OwnerUserID        uint64   `json:"owner_user_id,omitempty"`
	BotID              string   `json:"bot_id"`
	UserID             uint64   `json:"user_id"`
	Platform           string   `json:"platform"`
	PlatformServiceKey string   `json:"platform_service_key"`
	BindingID          uint64   `json:"binding_id,omitempty"`
	ProfileID          uint64   `json:"profile_id,omitempty"`
	GrantVersion       uint64   `json:"grant_version,omitempty"`
	PlatformAccountID  string   `json:"platform_account_id,omitempty"`
	Scopes             []string `json:"scopes"`
	jwt.RegisteredClaims
}

// TicketService mints HS256 service tickets shared with downstream
// game-services (genshin, mihomo) per Path D §1.5.
//
// signingKey is the deployment-wide SHARED_TICKET_KEY: the SAME key
// account-center uses to sign OAuth access tokens. The two contexts
// are kept distinct by the iss claim — OAuth tokens use
// iss=account-center, service tickets use the configured issuer
// (typically paigram-account-center). Verifiers check iss against an
// allowlist before accepting any claim.
type TicketService struct {
	issuer     string
	ttl        time.Duration
	signingKey []byte
}

// NewTicketService validates config and constructs a TicketService.
//
// Returns ErrInvalidTicketConfig when:
//   - signingKey is empty or shorter than 32 bytes;
//   - service_ticket_ttl is non-positive.
//
// The 32-byte minimum mirrors the validation in
// internal/config/config.go::validateServiceTicketConfig.
func NewTicketService(authCfg config.AuthConfig, signingKey []byte) (*TicketService, error) {
	if authCfg.ServiceTicketTTLSeconds <= 0 {
		return nil, ErrInvalidTicketConfig
	}
	if len(signingKey) < 32 {
		return nil, ErrSigningKeyUnavailable
	}

	issuer := authCfg.ServiceTicketIssuer
	if issuer == "" {
		issuer = "paigram-account-center"
	}

	// Copy the key so callers cannot mutate our reference.
	key := make([]byte, len(signingKey))
	copy(key, signingKey)

	return &TicketService{
		issuer:     issuer,
		ttl:        time.Duration(authCfg.ServiceTicketTTLSeconds) * time.Second,
		signingKey: key,
	}, nil
}

// Issue mints a service ticket for one Dispatch call. The HS256 signature
// is over (header || payload) using the shared key. No kid header — Path
// D has exactly one signing key, so kid would be noise.
func (s *TicketService) Issue(botID, consumer string, binding *model.PlatformAccountBinding, scopes []string, audience string, profileID, grantVersion uint64) (string, time.Time, error) {
	if binding == nil || binding.Status != model.PlatformAccountBindingStatusActive {
		return "", time.Time{}, ErrInactiveAccountRef
	}
	if consumer == "" || audience == "" {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if grantVersion == 0 {
		return "", time.Time{}, ErrInvalidTicketConfig
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)
	claims := ServiceTicketClaims{
		Type:               "service_ticket",
		ActorType:          "consumer",
		ActorID:            consumer,
		Consumer:           consumer,
		ClientID:           botID,
		OwnerUserID:        binding.OwnerUserID,
		BotID:              botID,
		UserID:             binding.OwnerUserID,
		Platform:           binding.Platform,
		PlatformServiceKey: binding.PlatformServiceKey,
		BindingID:          binding.ID,
		ProfileID:          profileID,
		GrantVersion:       grantVersion,
		PlatformAccountID:  nullableBindingExternalAccountKey(binding.ExternalAccountKey),
		Scopes:             scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   fmt.Sprintf("user:%d", binding.OwnerUserID),
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        fmt.Sprintf("%s:%s:%d:%d", consumer, botID, binding.ID, now.UnixNano()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "service_ticket"
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

// DecodeGrantScopes parses a consumer_grants.scopes_json column value
// into a string slice. Returns an empty slice when the JSON is empty.
func DecodeGrantScopes(grant model.ConsumerGrant) ([]string, error) {
	if grant.ScopesJSON == "" {
		return []string{}, nil
	}

	var scopes []string
	if err := json.Unmarshal([]byte(grant.ScopesJSON), &scopes); err != nil {
		return nil, err
	}

	return scopes, nil
}
