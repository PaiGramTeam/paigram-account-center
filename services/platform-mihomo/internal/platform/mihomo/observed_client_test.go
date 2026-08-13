package mihomo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestObservedClientTracksOnlyOperationalDegradation(t *testing.T) {
	observer := &recordingUpstreamObserver{}
	client := NewObservedClient(&observedClientStub{err: &UpstreamError{Kind: ErrorUnavailable}}, observer)
	_, _, _, _ = client.ValidateAndDiscover(context.Background(), "credential", "region")
	assert.Equal(t, observedResult{operation: "discover", degraded: true}, observer.last)

	client = NewObservedClient(&observedClientStub{err: &UpstreamError{Kind: ErrorInvalidCredential}}, observer)
	_, _, _, _ = client.ValidateAndDiscover(context.Background(), "credential", "region")
	assert.Equal(t, observedResult{operation: "discover", degraded: false}, observer.last)
}

type observedResult struct {
	operation string
	degraded  bool
}

type recordingUpstreamObserver struct {
	last observedResult
}

func (o *recordingUpstreamObserver) RecordUpstreamResult(operation string, degraded bool) {
	o.last = observedResult{operation: operation, degraded: degraded}
}

type observedClientStub struct {
	err error
}

func (s *observedClientStub) ValidateAndDiscover(context.Context, string, string) (string, string, []DiscoveredProfile, error) {
	return "", "", nil, s.err
}

func (s *observedClientStub) RefreshCredential(context.Context, string, string) (RefreshResult, error) {
	return RefreshResult{}, s.err
}

func (s *observedClientStub) IssueAuthKey(context.Context, string, string) (string, int64, error) {
	return "", 0, s.err
}

func (s *observedClientStub) IssueAuthKeyWithTTL(context.Context, string, string, time.Duration) (string, int64, error) {
	return "", 0, s.err
}

func (s *observedClientStub) RevokeAuthKey(context.Context, string) error {
	return s.err
}
