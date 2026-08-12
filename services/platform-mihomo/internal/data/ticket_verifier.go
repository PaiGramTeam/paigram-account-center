package data

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	contractticket "github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
	"github.com/golang-jwt/jwt/v5"

	"platform-mihomo-service/internal/biz"
)

var ErrGrantVersionRevoked = errors.New("service ticket grant version revoked")

const maximumServiceTicketTTL = 5 * time.Minute

type TicketVerifier struct {
	issuer             string
	resolver           PublicKeyResolver
	lookup             GrantVersionLookup
	authorizationState AuthorizationStateLookup
}

type AuthorizationState struct {
	MinimumGrantVersion  uint64
	MinimumOwnerEpoch    uint64
	MinimumConsumerEpoch uint64
	MinimumEntryEpoch    uint64
	CredentialGeneration uint64
}

type AuthorizationStateLookup interface {
	LookupAuthorizationState(ctx context.Context, bindingRef, consumer string) (AuthorizationState, error)
}

type GrantVersionLookup interface {
	MinimumVersion(ctx context.Context, bindingRef string, consumer string) (uint64, error)
}

type serviceTicketJWTClaims struct {
	ActorType            string   `json:"actor_type"`
	ActorID              string   `json:"actor_id"`
	OwnerUserRef         string   `json:"owner_user_ref"`
	EntryIdentityRef     string   `json:"entry_identity_ref"`
	BindingRef           string   `json:"binding_ref"`
	Platform             string   `json:"platform"`
	AccountKey           string   `json:"account_key"`
	Consumer             string   `json:"consumer"`
	ConsumerPrincipal    string   `json:"consumer_principal"`
	GrantVersion         uint64   `json:"grant_version"`
	OwnerEpoch           uint64   `json:"owner_epoch"`
	ConsumerEpoch        uint64   `json:"consumer_epoch"`
	EntryEpoch           uint64   `json:"entry_epoch"`
	CredentialGeneration uint64   `json:"credential_generation"`
	OperationID          string   `json:"operation_id"`
	ProfileRef           string   `json:"profile_ref"`
	Scopes               []string `json:"scopes"`
	AllowedActions       []string `json:"allowed_actions"`
	BotID                string   `json:"bot_id"`
	PlatformServiceKey   string   `json:"platform_service_key"`
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

func (v *TicketVerifier) WithAuthorizationStateLookup(lookup AuthorizationStateLookup) *TicketVerifier {
	v.authorizationState = lookup
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
	if claims.BindingRef == "" {
		return nil, fmt.Errorf("service ticket missing binding_ref")
	}
	if claims.Platform == "" {
		return nil, fmt.Errorf("service ticket missing platform")
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != expectedAudience {
		return nil, fmt.Errorf("service ticket audience must be a single exact value")
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) <= 0 || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) > maximumServiceTicketTTL {
		return nil, fmt.Errorf("service ticket lifetime exceeds the allowed profile")
	}
	if claims.ActorType == "consumer" {
		if ticketType != contractticket.TypeDelegation {
			return nil, fmt.Errorf("consumer ticket must use delegation typ")
		}
		if claims.Consumer == "" || claims.ConsumerPrincipal == "" || claims.Consumer != claims.ConsumerPrincipal {
			return nil, fmt.Errorf("service ticket missing consumer principal")
		}
		if claims.OwnerUserRef == "" || claims.EntryIdentityRef == "" {
			return nil, fmt.Errorf("service ticket missing stable identity reference")
		}
		if claims.GrantVersion == 0 || claims.OwnerEpoch == 0 || claims.ConsumerEpoch == 0 || claims.EntryEpoch == 0 || claims.CredentialGeneration == 0 {
			return nil, fmt.Errorf("service ticket missing authorization version")
		}
		if claims.Subject != "consumer:"+claims.Consumer {
			return nil, fmt.Errorf("service ticket subject does not match consumer")
		}
		if v.lookup != nil {
			minimum, err := v.lookup.MinimumVersion(ctx, claims.BindingRef, claims.Consumer)
			if err != nil {
				return nil, err
			}
			if minimum > 0 && claims.GrantVersion < minimum {
				return nil, ErrGrantVersionRevoked
			}
		}
		if v.authorizationState != nil {
			state, err := v.authorizationState.LookupAuthorizationState(ctx, claims.BindingRef, claims.Consumer)
			if err != nil {
				return nil, err
			}
			if claims.GrantVersion < state.MinimumGrantVersion ||
				claims.OwnerEpoch < state.MinimumOwnerEpoch ||
				claims.ConsumerEpoch < state.MinimumConsumerEpoch ||
				claims.EntryEpoch < state.MinimumEntryEpoch ||
				claims.CredentialGeneration != state.CredentialGeneration {
				return nil, ErrGrantVersionRevoked
			}
		}
	} else {
		if ticketType != contractticket.TypeControl {
			return nil, fmt.Errorf("non-consumer ticket must use control typ")
		}
		switch claims.ActorType {
		case "user", "admin":
			if claims.OwnerUserRef == "" {
				return nil, fmt.Errorf("service ticket missing owner_user_ref")
			}
			if claims.Subject != "user:"+claims.OwnerUserRef {
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
	actions := claims.Scopes
	if len(claims.AllowedActions) > 0 {
		actions = claims.AllowedActions
	}

	return &biz.ServiceTicketClaims{
		TicketType:           ticketType,
		ActorType:            claims.ActorType,
		ActorID:              claims.ActorID,
		OwnerUserRef:         claims.OwnerUserRef,
		EntryIdentityRef:     claims.EntryIdentityRef,
		BindingRef:           claims.BindingRef,
		Platform:             claims.Platform,
		AccountKey:           claims.AccountKey,
		Consumer:             claims.Consumer,
		GrantVersion:         claims.GrantVersion,
		OwnerEpoch:           claims.OwnerEpoch,
		ConsumerEpoch:        claims.ConsumerEpoch,
		EntryEpoch:           claims.EntryEpoch,
		CredentialGeneration: claims.CredentialGeneration,
		OperationID:          claims.OperationID,
		ProfileRef:           claims.ProfileRef,
		Scopes:               actions,
		Audience:             expectedAudience,
		BotID:                claims.BotID,
		PlatformServiceKey:   claims.PlatformServiceKey,
	}, nil
}
