package botroute

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServiceGroup aggregates botroute services for DI into handler / gRPC
// initialization. Matches the enter.go pattern used elsewhere in this repo.
type ServiceGroup struct {
	Service Service
}

// NewServiceGroup creates the botroute service group.
func NewServiceGroup(db *gorm.DB, logger *zap.Logger) *ServiceGroup {
	return &ServiceGroup{Service: *NewService(db, logger)}
}
