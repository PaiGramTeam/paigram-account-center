package platformbinding

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"paigram/internal/response"
	serviceplatformbinding "paigram/internal/service/platformbinding"
)

type operationRecoveryService interface {
	ListForBinding(context.Context, uint64, serviceplatformbinding.ListParams) ([]serviceplatformbinding.OperationRecoveryRecord, int64, error)
	RequeueDeadLetter(context.Context, uint64, string, uint64) (*serviceplatformbinding.OperationRecoveryRecord, error)
}

func (h *AdminHandler) ListOperations(c *gin.Context) {
	bindingID, ok := parseBindingID(c)
	if !ok {
		return
	}
	if h.operationRecovery == nil {
		response.InternalServerError(c, "operation recovery is unavailable")
		return
	}
	if _, err := h.bindingService.GetBindingByID(bindingID); err != nil {
		writeBindingError(c, err, "failed to get platform binding")
		return
	}
	page, pageSize := parseListParams(c)
	items, total, err := h.operationRecovery.ListForBinding(c.Request.Context(), bindingID, serviceplatformbinding.ListParams{Page: page, PageSize: pageSize})
	if err != nil {
		writeBindingError(c, err, "failed to list platform operations")
		return
	}
	response.SuccessWithPagination(c, buildOperationRecoveryViews(items), total, page, pageSize)
}

func (h *AdminHandler) RequeueOperation(c *gin.Context) {
	bindingID, ok := parseBindingID(c)
	if !ok {
		return
	}
	operationID := strings.TrimSpace(c.Param("operationId"))
	if operationID == "" || len(operationID) > 64 {
		response.BadRequest(c, "invalid operation id")
		return
	}
	adminUserID, ok := currentUserID(c)
	if !ok {
		return
	}
	if h.operationRecovery == nil {
		response.InternalServerError(c, "operation recovery is unavailable")
		return
	}
	record, err := h.operationRecovery.RequeueDeadLetter(c.Request.Context(), bindingID, operationID, adminUserID)
	if err != nil {
		writeBindingError(c, err, "failed to requeue platform operation")
		return
	}
	response.Success(c, buildOperationRecoveryView(record))
}
