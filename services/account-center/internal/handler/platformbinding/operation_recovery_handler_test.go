package platformbinding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/middleware"
	"paigram/internal/model"
	serviceplatformbinding "paigram/internal/service/platformbinding"
)

type operationRecoveryStub struct {
	items       []serviceplatformbinding.OperationRecoveryRecord
	total       int64
	requeued    *serviceplatformbinding.OperationRecoveryRecord
	bindingID   uint64
	operationID string
	adminUserID uint64
}

func (s *operationRecoveryStub) ListForBinding(_ context.Context, bindingID uint64, _ serviceplatformbinding.ListParams) ([]serviceplatformbinding.OperationRecoveryRecord, int64, error) {
	s.bindingID = bindingID
	return s.items, s.total, nil
}

func (s *operationRecoveryStub) RequeueDeadLetter(_ context.Context, bindingID uint64, operationID string, adminUserID uint64) (*serviceplatformbinding.OperationRecoveryRecord, error) {
	s.bindingID = bindingID
	s.operationID = operationID
	s.adminUserID = adminUserID
	return s.requeued, nil
}

func TestAdminListsSafeOperationRecoveryState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recovery := &operationRecoveryStub{items: []serviceplatformbinding.OperationRecoveryRecord{{
		OperationID: "op_dead", Kind: "OPERATION_KIND_BIND_CREDENTIAL",
		State: model.PlatformOperationIntentStateInvariantViolation, ReasonCode: "resolve_rejected",
		OutboxStatus: model.PlatformOperationOutboxStatusDeadLetter, AttemptCount: 100, AvailableAt: time.Now().UTC(),
	}}, total: 1}
	handler := NewAdminHandler(mutationBindingStub{}, &mutationProfileStub{}, &mutationGrantStub{}, &mutationOrchestrationStub{}, mutationRuntimeSummaryStub{}, recovery)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "bindingId", Value: "101"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-accounts/101/operations", nil)
	handler.ListOperations(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint64(101), recovery.bindingID)
	assert.Contains(t, w.Body.String(), `"operation_id":"op_dead"`)
	assert.Contains(t, w.Body.String(), `"outbox_status":"dead_letter"`)
	assert.NotContains(t, w.Body.String(), "request_fingerprint")
	assert.NotContains(t, w.Body.String(), "actor_id")
}

func TestAdminRequeuesDeadLetterWithAuthenticatedActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recovery := &operationRecoveryStub{requeued: &serviceplatformbinding.OperationRecoveryRecord{
		OperationID: "op_dead", State: model.PlatformOperationIntentStateInvariantViolation,
		OutboxStatus: model.PlatformOperationOutboxStatusPending,
	}}
	handler := NewAdminHandler(mutationBindingStub{}, &mutationProfileStub{}, &mutationGrantStub{}, &mutationOrchestrationStub{}, mutationRuntimeSummaryStub{}, recovery)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = []gin.Param{{Key: "bindingId", Value: "101"}, {Key: "operationId", Value: "op_dead"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/platform-accounts/101/operations/op_dead/requeue", nil)
	middleware.SetUserID(c, 99)
	handler.RequeueOperation(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, uint64(101), recovery.bindingID)
	assert.Equal(t, "op_dead", recovery.operationID)
	assert.Equal(t, uint64(99), recovery.adminUserID)
	assert.Contains(t, w.Body.String(), `"outbox_status":"pending"`)
}
