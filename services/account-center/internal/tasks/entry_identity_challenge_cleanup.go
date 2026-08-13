package tasks

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"paigram/internal/model"
)

const TypeCleanExpiredEntryIdentityChallenges = "entry_identity:clean_expired_challenges"

type CleanExpiredEntryIdentityChallengesHandler struct {
	db *gorm.DB
}

func NewCleanExpiredEntryIdentityChallengesTask() *asynq.Task {
	return asynq.NewTask(TypeCleanExpiredEntryIdentityChallenges, nil)
}

func NewCleanExpiredEntryIdentityChallengesHandler(db *gorm.DB) *CleanExpiredEntryIdentityChallengesHandler {
	return &CleanExpiredEntryIdentityChallengesHandler{db: db}
}

func (h *CleanExpiredEntryIdentityChallengesHandler) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil {
		return fmt.Errorf("clean expired entry identity challenges: database is required")
	}
	if err := h.db.WithContext(ctx).
		Where("expires_at <= CURRENT_TIMESTAMP").
		Delete(&model.EntryIdentityLinkChallenge{}).Error; err != nil {
		return fmt.Errorf("clean expired entry identity challenges: %w", err)
	}
	return nil
}
