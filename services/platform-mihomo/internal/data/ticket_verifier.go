package data

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strconv"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"

	"platform-mihomo-service/internal/biz"
)

var ErrGrantVersionRevoked = errors.New("service ticket grant version revoked")

type TicketVerifier struct {
	issuer   string
	resolver PublicKeyResolver
	lookup   GrantVersionLookup
}

type GrantVersionLookup interface {
	MinimumVersion(ctx context.Context, bindingID uint64, consumer string) (uint64, error)
}

type serviceTicketJWTClaims struct {
	ActorType            string   `json:"actor_type"`
	ActorID              string   `json:"actor_id"`
	OwnerUserID          uint64   `json:"owner_user_id"`
	BindingID            uint64   `json:"binding_id"`
	Platform             string   `json:"platform"`
	PlatformAccountID    string   `json:"platform_account_id"`
	Consumer             string   `json:"consumer"`
	GrantVersion         uint64   `json:"grant_version"`
	ProfileID            uint64   `json:"profile_id"`
	Scopes               []string `json:"scopes"`
	AllowedActions       []string `json:"allowed_actions"`
	Audience             string   `json:"audience"`
	BotID                string   `json:"bot_id"`
	UserID               uint64   `json:"user_id"`
	PlatformServiceKey   string   `json:"platform_service_key"`
	PlatformAccountRefID uint64   `json:"platform_account_ref_id"`
	TicketType           string   `json:"typ"`
	jwt.RegisteredClaims
}

func NewTicketVerifier(issuer string, key []byte) *TicketVerifier {
	return NewStaticKeyTicketVerifier(issuer, "default", ed25519.PublicKey(key))
}

func NewTicketVerifierWithResolver(issuer string, resolver PublicKeyResolver) *TicketVerifier {
	return &TicketVerifier{issuer: issuer, resolver: resolver}
}

func NewStaticKeyTicketVerifier(issuer, kid string, publicKey ed25519.PublicKey) *TicketVerifier {
	return NewTicketVerifierWithResolver(issuer, NewStaticPublicKeyResolver(kid, publicKey))
}

func (v *TicketVerifier) WithGrantVersionLookup(lookup GrantVersionLookup) *TicketVerifier {
	v.lookup = lookup
	return v
}

func (v *TicketVerifier) Verify(raw string, expectedAudience string) (*biz.ServiceTicketClaims, error) {
	return v.VerifyContext(context.Background(), raw, expectedAudience)
}

func (v *TicketVerifier) VerifyContext(ctx context.Context, raw string, expectedAudience string) (*biz.ServiceTicketClaims, error) {
	claims := &serviceTicketJWTClaims{}
	ticketType := ""

	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != contractticket.AlgorithmEd25519 {
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("service ticket missing kid")
		}
		typ, ok := token.Header["typ"].(string)
		if !ok || (typ != contractticket.TypeControl && typ != contractticket.TypeDelegation) {
			return nil, fmt.Errorf("invalid service ticket typ")
		}
		ticketType = typ
		if v.resolver == nil {
			return nil, fmt.Errorf("service ticket public key resolver is not configured")
		}
		return v.resolver.Resolve(ctx, kid)
	}, jwt.WithValidMethods([]string{contractticket.AlgorithmEd25519}), jwt.WithAudience(expectedAudience), jwt.WithIssuer(v.issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid service ticket")
	}
	if claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil || claims.NotBefore == nil {
		return nil, fmt.Errorf("service ticket missing required registered claim")
	}
	if claims.ActorType == "" {
		return nil, fmt.Errorf("service ticket missing actor_type")
	}
	if claims.ActorID == "" {
		return nil, fmt.Errorf("service ticket missing actor_id")
	}
	if claims.OwnerUserID == 0 {
		return nil, fmt.Errorf("service ticket missing owner_user_id")
	}
	if claims.BindingID == 0 {
		return nil, fmt.Errorf("service ticket missing binding_id")
	}
	if claims.PlatformAccountRefID != 0 && claims.PlatformAccountRefID != claims.BindingID {
		return nil, fmt.Errorf("service ticket binding_id does not match platform_account_ref_id")
	}
	if claims.Platform == "" {
		return nil, fmt.Errorf("service ticket missing platform")
	}
	if claims.ActorType == "consumer" {
		if ticketType != contractticket.TypeDelegation {
			return nil, fmt.Errorf("consumer ticket must use delegation typ")
		}
		if claims.Consumer == "" {
			return nil, fmt.Errorf("service ticket missing consumer")
		}
		if claims.GrantVersion == 0 {
			return nil, fmt.Errorf("service ticket missing grant_version")
		}
		if claims.Subject != "consumer:"+claims.Consumer {
			return nil, fmt.Errorf("service ticket subject does not match consumer")
		}
		if v.lookup != nil {
			minimum, err := v.lookup.MinimumVersion(ctx, claims.BindingID, claims.Consumer)
			if err != nil {
				return nil, err
			}
			if minimum > 0 && claims.GrantVersion < minimum {
				return nil, ErrGrantVersionRevoked
			}
		}
	} else {
		if ticketType != contractticket.TypeControl {
			return nil, fmt.Errorf("non-consumer ticket must use control typ")
		}
		switch claims.ActorType {
		case "user", "admin":
			if claims.Subject != "user:"+strconv.FormatUint(claims.OwnerUserID, 10) {
				return nil, fmt.Errorf("service ticket subject does not match owner")
			}
		case "system":
			if claims.Subject != "system:account-center" {
				return nil, fmt.Errorf("service ticket subject does not match system actor")
			}
		default:
			return nil, fmt.Errorf("unsupported service ticket actor_type")
		}
	}
	userID := claims.OwnerUserID
	if claims.UserID != 0 {
		userID = claims.UserID
	}
	actions := claims.Scopes
	if len(claims.AllowedActions) > 0 {
		actions = claims.AllowedActions
	}

	return &biz.ServiceTicketClaims{
		TicketType:         ticketType,
		ActorType:          claims.ActorType,
		ActorID:            claims.ActorID,
		OwnerUserID:        claims.OwnerUserID,
		BindingID:          claims.BindingID,
		Platform:           claims.Platform,
		PlatformAccountID:  claims.PlatformAccountID,
		Consumer:           claims.Consumer,
		GrantVersion:       claims.GrantVersion,
		ProfileID:          claims.ProfileID,
		Scopes:             actions,
		Audience:           expectedAudience,
		BotID:              claims.BotID,
		UserID:             userID,
		PlatformServiceKey: claims.PlatformServiceKey,
		// Keep the legacy alias available to downstream code while requiring
		// any ticket-level platform_account_ref_id claim to match binding_id.
		PlatformAccountRefID: claims.BindingID,
	}, nil
}
