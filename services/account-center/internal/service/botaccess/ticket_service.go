package botaccess

import (
	"slices"
	"time"

	"paigram/internal/config"
	"paigram/internal/model"
	"paigram/internal/serviceticket"
)

type ServiceTicketClaims = serviceticket.Claims

type TicketService struct {
	signer *serviceticket.Signer
}

type DelegationAuthorization struct {
	OwnerUserRef     string
	EntryIdentityRef string
	OwnerEpoch       uint64
	ConsumerEpoch    uint64
	EntryEpoch       uint64
}

func NewTicketService(authCfg config.AuthConfig) (*TicketService, error) {
	signer, err := serviceticket.NewSigner(serviceticket.Config{
		Issuer:        authCfg.ServiceTicketIssuer,
		KeyID:         authCfg.ServiceTicketKeyID,
		TTL:           time.Duration(authCfg.ServiceTicketTTLSeconds) * time.Second,
		PrivateKeyPEM: authCfg.ServiceTicketPrivateKeyPEM,
	})
	if err != nil {
		return nil, ErrInvalidTicketConfig
	}
	return &TicketService{signer: signer}, nil
}

func (s *TicketService) Issue(botID, consumer string, binding *model.PlatformAccountBinding, action string, audience string, profileRef string, grantVersion uint64, authorization DelegationAuthorization) (string, time.Time, error) {
	if binding == nil || binding.Status != model.PlatformAccountBindingStatusActive {
		return "", time.Time{}, ErrInactiveBinding
	}
	if consumer == "" || audience == "" || binding.BindingRef == "" || !binding.ExternalAccountKey.Valid || binding.ExternalAccountKey.String == "" ||
		authorization.OwnerUserRef == "" || authorization.EntryIdentityRef == "" || authorization.OwnerEpoch == 0 || authorization.ConsumerEpoch == 0 || authorization.EntryEpoch == 0 || binding.Generation == 0 {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if grantVersion == 0 {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if action == "" {
		return "", time.Time{}, ErrInvalidTicketConfig
	}

	claims := ServiceTicketClaims{
		ActorType:            "consumer",
		ActorID:              consumer,
		Consumer:             consumer,
		ConsumerPrincipal:    consumer,
		ClientID:             consumer,
		OwnerUserRef:         authorization.OwnerUserRef,
		EntryIdentityRef:     authorization.EntryIdentityRef,
		BotID:                botID,
		Platform:             binding.Platform,
		PlatformServiceKey:   binding.PlatformServiceKey,
		BindingRef:           binding.BindingRef,
		ProfileRef:           profileRef,
		GrantVersion:         grantVersion,
		OwnerEpoch:           authorization.OwnerEpoch,
		ConsumerEpoch:        authorization.ConsumerEpoch,
		EntryEpoch:           authorization.EntryEpoch,
		CredentialGeneration: binding.Generation,
		AccountKey:           nullableBindingExternalAccountKey(binding.ExternalAccountKey),
		Scopes:               []string{action},
		AllowedActions:       []string{action},
	}

	return s.signer.Issue(serviceticket.TypeDelegation, "consumer:"+consumer, audience, claims)
}

func GrantActions(grant model.ConsumerGrant) []string {
	scopes := make([]string, 0, len(grant.Actions))
	for _, action := range grant.Actions {
		scopes = append(scopes, action.Action)
	}
	slices.Sort(scopes)
	return scopes
}
