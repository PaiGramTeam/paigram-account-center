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
	TypeCredentialOperationDispatch  = "platform_binding:credential_operation_dispatch"
	TypeCredentialOperationReconcile = "platform_binding:credential_operation_reconcile"
	credentialOperationDispatchLimit = 100
)

type credentialOperationTaskPayload struct {
	OperationID string `json:"operation_id,omitempty"`
}

type credentialOperationDueScanner interface {
	ClaimDueOperationIDs(context.Context, time.Time, int) ([]string, error)
	Reschedule(context.Context, string, string, time.Time) error
}

type credentialOperationReconciler interface {
	ReconcileCredentialOperation(context.Context, string) error
}

func NewCredentialOperationDispatchTask() (*asynq.Task, error) {
	return newCredentialOperationTask(TypeCredentialOperationDispatch, "")
}

func NewCredentialOperationReconcileTask(operationID string) (*asynq.Task, error) {
	if operationID == "" {
		return nil, serviceplatformbinding.ErrInvalidBindingMutation
	}
	return newCredentialOperationTask(TypeCredentialOperationReconcile, operationID)
}

func newCredentialOperationTask(taskType, operationID string) (*asynq.Task, error) {
	payload, err := json.Marshal(credentialOperationTaskPayload{OperationID: operationID})
	if err != nil {
		return nil, fmt.Errorf("marshal credential operation task: %w", err)
	}
	return asynq.NewTask(taskType, payload, asynq.MaxRetry(8), asynq.Timeout(2*time.Minute)), nil
}

type CredentialOperationDispatchHandler struct {
	scanner  credentialOperationDueScanner
	enqueuer asynqEnqueuer
}

func NewCredentialOperationDispatchHandler(db *gorm.DB, enqueuer asynqEnqueuer) *CredentialOperationDispatchHandler {
	return &CredentialOperationDispatchHandler{scanner: serviceplatformbinding.NewOperationIntentService(db), enqueuer: enqueuer}
}

func NewCredentialOperationDispatchHandlerWithScanner(scanner credentialOperationDueScanner, enqueuer asynqEnqueuer) *CredentialOperationDispatchHandler {
	return &CredentialOperationDispatchHandler{scanner: scanner, enqueuer: enqueuer}
}

func (h *CredentialOperationDispatchHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if _, err := decodeCredentialOperationTaskPayload(task); err != nil {
		return err
	}
	operationIDs, err := h.scanner.ClaimDueOperationIDs(ctx, time.Now().UTC(), credentialOperationDispatchLimit)
	if err != nil {
		return err
	}
	for _, operationID := range operationIDs {
		reconcileTask, taskErr := NewCredentialOperationReconcileTask(operationID)
		if taskErr != nil {
			return taskErr
		}
		if _, enqueueErr := h.enqueuer.Enqueue(reconcileTask, asynq.Unique(55*time.Second)); enqueueErr != nil {
			if errors.Is(enqueueErr, asynq.ErrDuplicateTask) {
				continue
			}
			if releaseErr := h.scanner.Reschedule(ctx, operationID, "enqueue_failed", time.Now().UTC()); releaseErr != nil {
				return errors.Join(fmt.Errorf("enqueue credential operation %s: %w", operationID, enqueueErr), releaseErr)
			}
			return fmt.Errorf("enqueue credential operation %s: %w", operationID, enqueueErr)
		}
	}
	return nil
}

type CredentialOperationReconcileHandler struct {
	reconciler credentialOperationReconciler
}

func NewCredentialOperationReconcileHandler(db *gorm.DB, platformService *serviceplatform.PlatformService) *CredentialOperationReconcileHandler {
	bindingService := serviceplatformbinding.NewBindingService(db)
	profileService := serviceplatformbinding.NewProfileProjectionService(db)
	intentService := serviceplatformbinding.NewOperationIntentService(db)
	gateway := serviceplatformbinding.NewGRPCGenericCredentialGateway(platformService.ControlDialer())
	orchestrationService := serviceplatformbinding.NewOrchestrationService(bindingService, platformService, gateway, profileService, intentService)
	return &CredentialOperationReconcileHandler{reconciler: orchestrationService}
}

func NewCredentialOperationReconcileHandlerWithReconciler(reconciler credentialOperationReconciler) *CredentialOperationReconcileHandler {
	return &CredentialOperationReconcileHandler{reconciler: reconciler}
}

func (h *CredentialOperationReconcileHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCredentialOperationTaskPayload(task)
	if err != nil {
		return err
	}
	if payload.OperationID == "" {
		return serviceplatformbinding.ErrInvalidBindingMutation
	}
	return h.reconciler.ReconcileCredentialOperation(ctx, payload.OperationID)
}

func decodeCredentialOperationTaskPayload(task *asynq.Task) (credentialOperationTaskPayload, error) {
	var payload credentialOperationTaskPayload
	if task == nil {
		return payload, serviceplatformbinding.ErrInvalidBindingMutation
	}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("unmarshal credential operation task: %w", err)
	}
	return payload, nil
}
