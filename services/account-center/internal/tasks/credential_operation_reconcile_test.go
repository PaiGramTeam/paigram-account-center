package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCredentialOperationDueScanner struct {
	operationIDs []string
	err          error
	rescheduled  string
}

func (f *fakeCredentialOperationDueScanner) ClaimDueOperationIDs(context.Context, time.Time, int) ([]string, error) {
	return f.operationIDs, f.err
}

func (f *fakeCredentialOperationDueScanner) Reschedule(_ context.Context, operationID, _ string, _ time.Time) error {
	f.rescheduled = operationID
	return nil
}

type fakeCredentialOperationReconciler struct {
	operationID string
	err         error
}

func (f *fakeCredentialOperationReconciler) ReconcileCredentialOperation(_ context.Context, operationID string) error {
	f.operationID = operationID
	return f.err
}

func TestCredentialOperationDispatchEnqueuesPayloadFreeWakeups(t *testing.T) {
	scanner := &fakeCredentialOperationDueScanner{operationIDs: []string{"op_one", "op_two"}}
	enqueuer := &fakeAsynqEnqueuer{}
	handler := NewCredentialOperationDispatchHandlerWithScanner(scanner, enqueuer)
	task, err := NewCredentialOperationDispatchTask()
	require.NoError(t, err)

	require.NoError(t, handler.ProcessTask(context.Background(), task))
	require.Len(t, enqueuer.tasks, 2)
	for index, queued := range enqueuer.tasks {
		assert.Equal(t, TypeCredentialOperationReconcile, queued.Type())
		payload, decodeErr := decodeCredentialOperationTaskPayload(queued)
		require.NoError(t, decodeErr)
		assert.Equal(t, scanner.operationIDs[index], payload.OperationID)
		assert.NotContains(t, string(queued.Payload()), "credential")
		assert.NotContains(t, string(queued.Payload()), "cookie")
	}
}

func TestCredentialOperationDispatchReleasesClaimWhenEnqueueFails(t *testing.T) {
	scanner := &fakeCredentialOperationDueScanner{operationIDs: []string{"op_one"}}
	handler := NewCredentialOperationDispatchHandlerWithScanner(scanner, &fakeAsynqEnqueuer{err: errors.New("redis unavailable")})
	task, err := NewCredentialOperationDispatchTask()
	require.NoError(t, err)

	require.Error(t, handler.ProcessTask(context.Background(), task))
	assert.Equal(t, "op_one", scanner.rescheduled)
}

func TestCredentialOperationReconcileHandlerUsesOperationID(t *testing.T) {
	reconciler := &fakeCredentialOperationReconciler{}
	handler := NewCredentialOperationReconcileHandlerWithReconciler(reconciler)
	task, err := NewCredentialOperationReconcileTask("op_test")
	require.NoError(t, err)

	require.NoError(t, handler.ProcessTask(context.Background(), task))
	assert.Equal(t, "op_test", reconciler.operationID)
}

func TestCredentialOperationReconcileHandlerReturnsReconcilerError(t *testing.T) {
	expected := errors.New("retry reconciliation")
	reconciler := &fakeCredentialOperationReconciler{err: expected}
	handler := NewCredentialOperationReconcileHandlerWithReconciler(reconciler)
	task, err := NewCredentialOperationReconcileTask("op_test")
	require.NoError(t, err)

	require.ErrorIs(t, handler.ProcessTask(context.Background(), task), expected)
}

var _ asynq.Handler = asynq.HandlerFunc(NewCredentialOperationReconcileHandlerWithReconciler(&fakeCredentialOperationReconciler{}).ProcessTask)
