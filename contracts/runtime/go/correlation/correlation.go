package correlation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

const (
	RequestIDHeader   = "x-request-id"
	TraceParentHeader = "traceparent"
	OperationIDHeader = "x-operation-id"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
var traceParentPattern = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`)

type Fields struct {
	RequestID   string
	TraceParent string
	TraceID     string
	OperationID string
}

type contextKey struct{}

// IncomingFields converts transport header value lists into fail-closed correlation input.
func IncomingFields(requestIDs, traceParents, operationIDs []string) Fields {
	return Fields{
		RequestID:   onlyValue(requestIDs),
		TraceParent: onlyValue(traceParents),
		OperationID: onlyValue(operationIDs),
	}
}

func Ensure(ctx context.Context, incoming Fields) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	existing := FromContext(ctx)
	requestID := chooseIdentifier(incoming.RequestID, existing.RequestID)
	if requestID == "" {
		requestID = randomHex(16)
	}

	traceParent := chooseTraceParent(incoming.TraceParent, existing.TraceParent)
	if traceParent == "" {
		traceParent = newTraceParent()
	}

	operationID := chooseIdentifier(incoming.OperationID, existing.OperationID)
	fields := Fields{
		RequestID:   requestID,
		TraceParent: traceParent,
		TraceID:     traceID(traceParent),
		OperationID: operationID,
	}
	return context.WithValue(ctx, contextKey{}, fields)
}

func WithOperationID(ctx context.Context, operationID string) context.Context {
	ctx = Ensure(ctx, Fields{})
	fields := FromContext(ctx)
	fields.OperationID = validIdentifier(operationID)
	return context.WithValue(ctx, contextKey{}, fields)
}

func FromContext(ctx context.Context) Fields {
	if ctx == nil {
		return Fields{}
	}
	fields, _ := ctx.Value(contextKey{}).(Fields)
	return fields
}

func chooseIdentifier(incoming, existing string) string {
	if strings.TrimSpace(incoming) == "" {
		return validIdentifier(existing)
	}
	return validIdentifier(incoming)
}

func validIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if !identifierPattern.MatchString(value) {
		return ""
	}
	return value
}

func chooseTraceParent(incoming, existing string) string {
	if strings.TrimSpace(incoming) == "" {
		return validTraceParent(existing)
	}
	return validTraceParent(incoming)
}

func validTraceParent(value string) string {
	value = strings.TrimSpace(value)
	matches := traceParentPattern.FindStringSubmatch(value)
	if len(matches) != 4 || isAllZero(matches[1]) || isAllZero(matches[2]) {
		return ""
	}
	return value
}

func traceID(traceParent string) string {
	matches := traceParentPattern.FindStringSubmatch(traceParent)
	if len(matches) != 4 {
		return ""
	}
	return matches[1]
}

func newTraceParent() string {
	return "00-" + randomHex(16) + "-" + randomHex(8) + "-01"
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	for {
		_, _ = rand.Read(buffer)
		if !allZeroBytes(buffer) {
			return hex.EncodeToString(buffer)
		}
	}
}

func isAllZero(value string) bool {
	for _, character := range value {
		if character != '0' {
			return false
		}
	}
	return true
}

func allZeroBytes(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func onlyValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) != 1 {
		return "\x00"
	}
	return values[0]
}
