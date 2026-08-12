package healthcheck

import (
	"context"
	"errors"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// NewReadiness builds the readiness policy for Platform Mihomo's required local dependencies.
func NewReadiness(db *gorm.DB, redisClient *redis.Client) servicehealth.Checker {
	return servicehealth.NewReadiness(2*time.Second,
		servicehealth.Dependency{
			Name: "database",
			Check: func(ctx context.Context) error {
				if db == nil {
					return errors.New("database is required")
				}
				sqlDB, err := db.DB()
				if err != nil {
					return err
				}
				return sqlDB.PingContext(ctx)
			},
		},
		servicehealth.Dependency{
			Name: "redis",
			Check: func(ctx context.Context) error {
				if redisClient == nil {
					return errors.New("redis is required")
				}
				return redisClient.Ping(ctx).Err()
			},
		},
	)
}
