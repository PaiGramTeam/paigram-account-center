package platformbinding

import (
	serviceaudit "paigram/internal/service/audit"

	"gorm.io/gorm"
)

type ServiceGroup struct {
	BindingService           BindingService
	GrantService             GrantService
	ProfileProjectionService ProfileProjectionService
	OrchestrationService     OrchestrationService
	RuntimeSummaryService    RuntimeSummaryService
}

func NewServiceGroup(db *gorm.DB, platformService interface {
	orchestrationPlatformService
	runtimeSummaryPlatformService
}, dependencies ...any) *ServiceGroup {
	bindingService := NewBindingService(db)
	profileProjectionService := NewProfileProjectionService(db)
	grantDependencies := append([]any{platformService}, dependencies...)
	grantService := NewGrantService(db, grantDependencies...)
	auditService := serviceaudit.NewAuditService(db)
	gateway := credentialGateway(NewGRPCGenericCredentialGateway(nil))
	for _, dependency := range dependencies {
		if candidate, ok := dependency.(credentialGateway); ok {
			gateway = candidate
		}
	}
	return &ServiceGroup{
		BindingService:           *bindingService,
		GrantService:             *grantService,
		ProfileProjectionService: *profileProjectionService,
		OrchestrationService:     *NewOrchestrationService(bindingService, platformService, gateway, profileProjectionService, grantService, auditService),
		RuntimeSummaryService:    *NewRuntimeSummaryService(platformService, bindingService, profileProjectionService),
	}
}
