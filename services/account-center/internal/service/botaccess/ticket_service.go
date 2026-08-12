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

func (s *TicketService) Issue(botID, consumer string, binding *model.PlatformAccountBinding, scopes []string, audience string, profileID, grantVersion uint64) (string, time.Time, error) {
	if binding == nil || binding.Status != model.PlatformAccountBindingStatusActive {
		return "", time.Time{}, ErrInactiveBinding
	}
	if consumer == "" || audience == "" {
		return "", time.Time{}, ErrInvalidTicketConfig
	}
	if grantVersion == 0 {
		return "", time.Time{}, ErrInvalidTicketConfig
	}

	claims := ServiceTicketClaims{
		ActorType:          "consumer",
		ActorID:            consumer,
		Consumer:           consumer,
		ClientID:           consumer,
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
		AllowedActions:     scopes,
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
