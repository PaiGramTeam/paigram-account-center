package correlation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsurePreservesValidIncomingFields(t *testing.T) {
	ctx := Ensure(context.Background(), Fields{
		RequestID:   "request-123",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		OperationID: "operation:123",
	})

	assert.Equal(t, Fields{
		RequestID:   "request-123",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		TraceID:     "4bf92f3577b34da6a3ce929d0e0e4736",
		OperationID: "operation:123",
	}, FromContext(ctx))
}

func TestEnsureRejectsUnsafeIncomingFields(t *testing.T) {
	ctx := Ensure(context.Background(), Fields{
		RequestID:   "request\r\ninjected: true",
		TraceParent: "00-00000000000000000000000000000000-0000000000000000-01",
		OperationID: "operation secret=value",
	})

	fields := FromContext(ctx)
	assert.Regexp(t, `^[0-9a-f]{32}$`, fields.RequestID)
	assert.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-01$`, fields.TraceParent)
	assert.NotEqual(t, "00000000000000000000000000000000", fields.TraceID)
	assert.Empty(t, fields.OperationID)
}

func TestEnsureKeepsExistingContextWhenHeadersAreMissing(t *testing.T) {
	existing := Ensure(context.Background(), Fields{
		RequestID:   "request-existing",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
	})

	assert.Equal(t, FromContext(existing), FromContext(Ensure(existing, Fields{})))
}

func TestWithOperationIDReplacesOnlyValidOperationID(t *testing.T) {
	ctx := Ensure(context.Background(), Fields{RequestID: "request-123"})
	ctx = WithOperationID(ctx, "operation_456")
	assert.Equal(t, "operation_456", FromContext(ctx).OperationID)

	ctx = WithOperationID(ctx, "invalid operation")
	assert.Empty(t, FromContext(ctx).OperationID)
}

func TestEnsureGeneratesIndependentIdentifiers(t *testing.T) {
	first := FromContext(Ensure(context.Background(), Fields{}))
	second := FromContext(Ensure(context.Background(), Fields{}))

	require.NotEmpty(t, first.RequestID)
	require.NotEmpty(t, first.TraceParent)
	assert.NotEqual(t, first.RequestID, second.RequestID)
	assert.NotEqual(t, first.TraceParent, second.TraceParent)
}

func TestIncomingFieldsRejectsAmbiguousValues(t *testing.T) {
	fields := IncomingFields(
		[]string{"request-first", "request-second"},
		[]string{"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		nil,
	)
	ctx := Ensure(context.Background(), fields)

	assert.NotEqual(t, "request-first", FromContext(ctx).RequestID)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", FromContext(ctx).TraceID)
}
