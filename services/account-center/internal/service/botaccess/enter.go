package botaccess

import (
	"paigram/internal/config"
	"paigram/internal/serviceticket"

	"gorm.io/gorm"
)

// ServiceGroup aggregates bot access services.
type ServiceGroup struct {
	BindingAccessService BindingAccessService
	TicketService        TicketService
}

// NewServiceGroup creates the bot access service group.
func NewServiceGroup(db *gorm.DB, authCfg config.AuthConfig) (*ServiceGroup, error) {
	ticketService, err := NewTicketService(authCfg)
	if err != nil {
		return nil, err
	}

	return &ServiceGroup{
		BindingAccessService: BindingAccessService{db: db},
		TicketService:        *ticketService,
	}, nil
}

func NewServiceGroupWithSigner(db *gorm.DB, signer serviceticket.Signer) (*ServiceGroup, error) {
	ticketService, err := NewTicketServiceWithSigner(signer)
	if err != nil {
		return nil, err
	}
	return &ServiceGroup{
		BindingAccessService: BindingAccessService{db: db},
		TicketService:        *ticketService,
	}, nil
}
