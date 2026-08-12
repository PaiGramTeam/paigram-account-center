package model

import (
	"time"
)

// PlatformOperationIntentState is the Account Center view of a cross-service command.
type PlatformOperationIntentState string

type PlatformOperationDeliveryMode string

const (
	PlatformOperationIntentStatePendingDelivery    PlatformOperationIntentState = "pending_delivery"
	PlatformOperationIntentStateUncertain          PlatformOperationIntentState = "uncertain"
	PlatformOperationIntentStateProjectionPending  PlatformOperationIntentState = "projection_pending"
	PlatformOperationIntentStateInputRequired      PlatformOperationIntentState = "input_required"
	PlatformOperationIntentStateInvariantViolation PlatformOperationIntentState = "invariant_violation"
	PlatformOperationIntentStateSucceeded          PlatformOperationIntentState = "succeeded"
	PlatformOperationIntentStateFailed             PlatformOperationIntentState = "failed"
	PlatformOperationIntentStateSuperseded         PlatformOperationIntentState = "superseded"
)

const (
	PlatformOperationDeliveryModeSyncSecret PlatformOperationDeliveryMode = "sync_secret"
	PlatformOperationDeliveryModeOutbox     PlatformOperationDeliveryMode = "outbox"
)

func (state PlatformOperationIntentState) ReservesBinding() bool {
	switch state {
	case PlatformOperationIntentStatePendingDelivery,
		PlatformOperationIntentStateUncertain,
		PlatformOperationIntentStateProjectionPending,
		PlatformOperationIntentStateInputRequired,
		PlatformOperationIntentStateInvariantViolation:
		return true
	default:
		return false
	}
}

// PlatformOperationIntent contains only the non-sensitive tuple needed to reconcile a command.
type PlatformOperationIntent struct {
	OperationID        string                        `gorm:"primaryKey;size:64"`
	BindingID          uint64                        `gorm:"not null;index:idx_platform_operation_intents_binding_id"`
	BindingRef         string                        `gorm:"size:64;not null"`
	OwnerUserID        uint64                        `gorm:"not null;index:idx_platform_operation_intents_owner"`
	Platform           string                        `gorm:"size:64;not null"`
	Kind               string                        `gorm:"size:64;not null"`
	PreGeneration      uint64                        `gorm:"not null"`
	TargetGeneration   uint64                        `gorm:"not null"`
	RequestFingerprint string                        `gorm:"size:64;not null"`
	DeliveryMode       PlatformOperationDeliveryMode `gorm:"size:32;not null"`
	State              PlatformOperationIntentState  `gorm:"size:32;not null;index:idx_platform_operation_intents_state"`
	ReasonCode         string                        `gorm:"size:64"`
	ActorType          string                        `gorm:"size:32;not null"`
	ActorID            string                        `gorm:"size:191;not null"`
	InputExpiresAt     *time.Time
	ResolvedAt         *time.Time
	CreatedAt          time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt          time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (PlatformOperationIntent) TableName() string {
	return "platform_operation_intents"
}

type PlatformOperationOutboxStatus string

const (
	PlatformOperationOutboxStatusPending    PlatformOperationOutboxStatus = "pending"
	PlatformOperationOutboxStatusCompleted  PlatformOperationOutboxStatus = "completed"
	PlatformOperationOutboxStatusDeadLetter PlatformOperationOutboxStatus = "dead_letter"
)

// PlatformOperationOutbox is a payload-free wake-up record keyed by operation ID.
type PlatformOperationOutbox struct {
	ID             uint64                        `gorm:"primaryKey"`
	OperationID    string                        `gorm:"size:64;not null;uniqueIndex:uk_platform_operation_outbox_operation"`
	Status         PlatformOperationOutboxStatus `gorm:"size:32;not null;index:idx_platform_operation_outbox_due,priority:1"`
	AvailableAt    time.Time                     `gorm:"not null;index:idx_platform_operation_outbox_due,priority:2"`
	AttemptCount   uint32                        `gorm:"not null;default:0"`
	LastReasonCode string                        `gorm:"size:64"`
	CreatedAt      time.Time                     `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time                     `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (PlatformOperationOutbox) TableName() string {
	return "platform_operation_outbox"
}
