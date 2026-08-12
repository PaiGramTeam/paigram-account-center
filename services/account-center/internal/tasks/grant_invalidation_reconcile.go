package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	serviceplatform "paigram/internal/service/platform"
	serviceplatformbinding "paigram/internal/service/platformbinding"
)

const (
	TypeGrantInvalidationDispatch  = "platform_binding:grant_invalidation_dispatch"
	TypeGrantInvalidationReconcile = "platform_binding:grant_invalidation_reconcile"
	grantInvalidationDispatchLimit = 100
)

type grantInvalidationTaskPayload struct {
	GrantID uint64 `json:"grant_id,omitempty"`
}

type pendingGrantInvalidationScanner interface {
	ListPendingGrantInvalidationIDs(context.Context, int) ([]uint64, error)
}

type grantInvalidationReconciler interface {
	ReconcileGrantInvalidation(context.Context, uint64) error
}

func NewGrantInvalidationDispatchTask() (*asynq.Task, error) {
	return newGrantInvalidationTask(TypeGrantInvalidationDispatch, 0)
}

func NewGrantInvalidationReconcileTask(grantID uint64) (*asynq.Task, error) {
	if grantID == 0 {
		return nil, serviceplatformbinding.ErrInvalidBindingMutation
	}
	return newGrantInvalidationTask(TypeGrantInvalidationReconcile, grantID)
}

func newGrantInvalidationTask(taskType string, grantID uint64) (*asynq.Task, error) {
	payload, err := json.Marshal(grantInvalidationTaskPayload{GrantID: grantID})
	if err != nil {
		return nil, fmt.Errorf("marshal grant invalidation task: %w", err)
	}
	return asynq.NewTask(taskType, payload, asynq.MaxRetry(8), asynq.Timeout(2*time.Minute)), nil
}

type GrantInvalidationDispatchHandler struct {
	scanner  pendingGrantInvalidationScanner
	enqueuer asynqEnqueuer
}

func NewGrantInvalidationDispatchHandler(db *gorm.DB, enqueuer asynqEnqueuer) *GrantInvalidationDispatchHandler {
	return &GrantInvalidationDispatchHandler{scanner: serviceplatformbinding.NewGrantService(db), enqueuer: enqueuer}
}

func NewGrantInvalidationDispatchHandlerWithScanner(scanner pendingGrantInvalidationScanner, enqueuer asynqEnqueuer) *GrantInvalidationDispatchHandler {
	return &GrantInvalidationDispatchHandler{scanner: scanner, enqueuer: enqueuer}
}

func (h *GrantInvalidationDispatchHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if _, err := decodeGrantInvalidationTaskPayload(task); err != nil {
		return err
	}
	grantIDs, err := h.scanner.ListPendingGrantInvalidationIDs(ctx, grantInvalidationDispatchLimit)
	if err != nil {
		return err
	}
	for _, grantID := range grantIDs {
		reconcileTask, taskErr := NewGrantInvalidationReconcileTask(grantID)
		if taskErr != nil {
			return taskErr
		}
		if _, enqueueErr := h.enqueuer.Enqueue(reconcileTask, asynq.Unique(55*time.Second)); enqueueErr != nil {
			if errors.Is(enqueueErr, asynq.ErrDuplicateTask) {
				continue
			}
			return fmt.Errorf("enqueue grant invalidation %d: %w", grantID, enqueueErr)
		}
	}
	return nil
}

type GrantInvalidationReconcileHandler struct {
	reconciler grantInvalidationReconciler
}

func NewGrantInvalidationReconcileHandler(db *gorm.DB, platformService *serviceplatform.PlatformService) *GrantInvalidationReconcileHandler {
	return &GrantInvalidationReconcileHandler{reconciler: serviceplatformbinding.NewGrantService(db, platformService)}
}

func NewGrantInvalidationReconcileHandlerWithReconciler(reconciler grantInvalidationReconciler) *GrantInvalidationReconcileHandler {
	return &GrantInvalidationReconcileHandler{reconciler: reconciler}
}

func (h *GrantInvalidationReconcileHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeGrantInvalidationTaskPayload(task)
	if err != nil {
		return err
	}
	if payload.GrantID == 0 {
		return serviceplatformbinding.ErrInvalidBindingMutation
	}
	return h.reconciler.ReconcileGrantInvalidation(ctx, payload.GrantID)
}

func decodeGrantInvalidationTaskPayload(task *asynq.Task) (grantInvalidationTaskPayload, error) {
	var payload grantInvalidationTaskPayload
	if task == nil {
		return payload, serviceplatformbinding.ErrInvalidBindingMutation
	}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("unmarshal grant invalidation task: %w", err)
	}
	return payload, nil
}
