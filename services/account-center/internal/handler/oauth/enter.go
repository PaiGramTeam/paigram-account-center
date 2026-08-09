package oauth

import "paigram/internal/service/credentials"

// ApiGroup is the layered-handler aggregate for /oauth/* and
// /admin/service-credentials routes.
type ApiGroup struct {
	TokenHandler       TokenHandler
	CredentialsHandler CredentialsHandler
}

// NewApiGroup wires the handler group against a credentials service group.
func NewApiGroup(serviceGroup *credentials.ServiceGroup) *ApiGroup {
	return &ApiGroup{
		TokenHandler:       *NewTokenHandler(&serviceGroup.TokenService),
		CredentialsHandler: *NewCredentialsHandler(&serviceGroup.Service),
	}
}
