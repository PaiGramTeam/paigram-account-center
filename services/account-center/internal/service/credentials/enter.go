package credentials

import "gorm.io/gorm"

// ServiceGroup aggregates credentials services for the layered-service
// pattern (enter.go convention from AGENTS.md §2).
type ServiceGroup struct {
	Service      Service
	TokenService TokenService
}

// NewServiceGroup wires the credentials registry and the HS256 token
// service against gorm.DB + a TokenServiceConfig. Returns an error if
// cfg.SigningKey is shorter than the HS256 minimum (see NewTokenService).
func NewServiceGroup(db *gorm.DB, cfg TokenServiceConfig) (*ServiceGroup, error) {
	credentialsService := NewService(db)
	tokenService, err := NewTokenService(credentialsService, cfg)
	if err != nil {
		return nil, err
	}
	return &ServiceGroup{
		Service:      *credentialsService,
		TokenService: *tokenService,
	}, nil
}
