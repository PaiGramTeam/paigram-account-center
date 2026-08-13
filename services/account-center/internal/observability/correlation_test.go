package observability

import (
	"context"
	"testing"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	sentry "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
)

func TestApplyCorrelationScopeAddsSanitizedContext(t *testing.T) {
	ctx := correlation.Ensure(context.Background(), correlation.Fields{
		RequestID:   "request-sentry",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		OperationID: "operation-sentry",
	})
	scope := sentry.NewScope()
	applyCorrelationScope(scope, ctx)

	event := scope.ApplyToEvent(sentry.NewEvent(), nil, nil)
	context := event.Contexts["correlation"]
	assert.Equal(t, "request-sentry", context["request_id"])
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", context["trace_id"])
	assert.Equal(t, "operation-sentry", context["operation_id"])
	assert.NotContains(t, event.Tags, "request_id")
}
