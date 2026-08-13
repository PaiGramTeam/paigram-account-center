package service

import (
	"testing"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTicketRejectionRecorderReceivesBoundedSurfaceAndStatus(t *testing.T) {
	recorder := &recordingTicketRejection{}
	service := (&PlatformControlService{}).WithTicketRejectionRecorder(recorder)

	service.recordTicketRejection(
		platformv2.PlatformControlService_GetBindingState_FullMethodName,
		status.Error(codes.Unauthenticated, "ticket rejected"),
	)

	assert.Equal(t, "control", recorder.surface)
	assert.Equal(t, codes.Unauthenticated.String(), recorder.reason)
}

type recordingTicketRejection struct {
	surface string
	reason  string
}

func (r *recordingTicketRejection) RecordTicketRejection(surface, reason string) {
	r.surface = surface
	r.reason = reason
}
