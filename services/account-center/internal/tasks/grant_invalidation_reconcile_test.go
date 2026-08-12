package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePendingGrantInvalidationScanner struct {
	grantIDs []uint64
	err      error
}

func (f *fakePendingGrantInvalidationScanner) ListPendingGrantInvalidationIDs(context.Context, int) ([]uint64, error) {
	return f.grantIDs, f.err
}

type fakeGrantInvalidationReconciler struct {
	grantID uint64
	err     error
}

func (f *fakeGrantInvalidationReconciler) ReconcileGrantInvalidation(_ context.Context, grantID uint64) error {
	f.grantID = grantID
	return f.err
}

func TestGrantInvalidationDispatchEnqueuesGrantIdentifiers(t *testing.T) {
	scanner := &fakePendingGrantInvalidationScanner{grantIDs: []uint64{11, 12}}
	enqueuer := &fakeAsynqEnqueuer{}
	handler := NewGrantInvalidationDispatchHandlerWithScanner(scanner, enqueuer)
	task, err := NewGrantInvalidationDispatchTask()
	require.NoError(t, err)

	require.NoError(t, handler.ProcessTask(context.Background(), task))
	require.Len(t, enqueuer.tasks, 2)
	for index, queued := range enqueuer.tasks {
		assert.Equal(t, TypeGrantInvalidationReconcile, queued.Type())
		payload, decodeErr := decodeGrantInvalidationTaskPayload(queued)
		require.NoError(t, decodeErr)
		assert.Equal(t, scanner.grantIDs[index], payload.GrantID)
	}
}

func TestGrantInvalidationReconcileHandlerPropagatesFailure(t *testing.T) {
	expected := errors.New("platform unavailable")
	reconciler := &fakeGrantInvalidationReconciler{err: expected}
	handler := NewGrantInvalidationReconcileHandlerWithReconciler(reconciler)
	task, err := NewGrantInvalidationReconcileTask(11)
	require.NoError(t, err)

	require.ErrorIs(t, handler.ProcessTask(context.Background(), task), expected)
	assert.Equal(t, uint64(11), reconciler.grantID)
}
