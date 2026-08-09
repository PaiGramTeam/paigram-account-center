package model

import (
	"database/sql"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BotRoute is the registry row mapping (bot_id, platform) to a single active
// game-service endpoint. The unique key on (bot_id, platform) enforces the
// 1:1:1 topology between a bot, a platform, and its current service.
type BotRoute struct {
	ID              uint64       `gorm:"primaryKey"`
	BotID           string       `gorm:"column:bot_id;size:64;not null;uniqueIndex:uk_bot_routes_bot_platform,priority:1"`
	Platform        string       `gorm:"column:platform;size:32;not null;uniqueIndex:uk_bot_routes_bot_platform,priority:2"`
	ServiceID       string       `gorm:"column:service_id;size:64;not null"`
	Endpoint        string       `gorm:"column:endpoint;size:255;not null"`
	HandlersJSON    []byte       `gorm:"column:handlers_json;type:json;not null"`
	Version         string       `gorm:"column:version;size:32;not null"`
	LastHeartbeatAt sql.NullTime `gorm:"column:last_heartbeat_at;type:datetime(3)"`
	CreatedAt       time.Time    `gorm:"column:created_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt       time.Time    `gorm:"column:updated_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName pins the table name; this matches the migration in
// initialize/migrate/sql/000002_create_bot_routes.up.sql.
func (BotRoute) TableName() string { return "bot_routes" }

// BotRouteAudit is an append-only audit log of register/unregister actions
// taken against bot_routes. Failures to write here MUST NOT block the primary
// route operation; callers should log-and-continue.
type BotRouteAudit struct {
	ID        uint64    `gorm:"primaryKey"`
	BotID     string    `gorm:"column:bot_id;size:64;not null;index:idx_bot_route_audit_bot,priority:1"`
	Platform  string    `gorm:"column:platform;size:32;not null"`
	Action    string    `gorm:"column:action;size:16;not null"`
	Payload   []byte    `gorm:"column:payload;type:json;not null"`
	Actor     string    `gorm:"column:actor;size:64;not null"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime(3);not null;default:CURRENT_TIMESTAMP(3);index:idx_bot_route_audit_bot,priority:2"`
}

func (BotRouteAudit) TableName() string { return "bot_route_audit" }

// UpsertBotRoute inserts the given route or, when (bot_id, platform) already
// exists, updates service_id / endpoint / handlers_json / version /
// last_heartbeat_at on the existing row. Other columns (id, created_at) are
// preserved. The route argument is mutated in place to reflect the persisted
// row when a new record is created.
func UpsertBotRoute(db *gorm.DB, route *BotRoute) *gorm.DB {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "bot_id"}, {Name: "platform"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"service_id",
			"endpoint",
			"handlers_json",
			"version",
			"last_heartbeat_at",
			"updated_at",
		}),
	}).Create(route)
}
