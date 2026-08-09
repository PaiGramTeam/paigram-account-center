package botaccess

import (
	"paigram/internal/config"

	"gorm.io/gorm"
)

// ServiceGroup aggregates bot access services.
type ServiceGroup struct {
	AccountRefService AccountRefService
	TicketService     TicketService
}

// NewServiceGroup creates the bot access service group. The HS256
// signingKey is the deployment-wide SHARED_TICKET_KEY (see
// internal/config.AuthConfig.ServiceTicketSigningKey) — the same key
// account-center uses for OAuth access tokens.
func NewServiceGroup(db *gorm.DB, authCfg config.AuthConfig, signingKey []byte) (*ServiceGroup, error) {
	ticketService, err := NewTicketService(authCfg, signingKey)
	if err != nil {
		return nil, err
	}

	return &ServiceGroup{
		AccountRefService: AccountRefService{db: db},
		TicketService:     *ticketService,
	}, nil
}
