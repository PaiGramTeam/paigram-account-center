package adminaudit

import (
	"net/http"

	"paigram/internal/httpserver"
	"paigram/internal/response"
	serviceaudit "paigram/internal/service/audit"
)

func registerContracts(rg *httpserver.Group) {
	errors := []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusInternalServerError,
	}
	rg.RegisterContract(http.MethodGet, "/admin/audit-logs", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[serviceaudit.AuditEventView]]{}, http.StatusOK, errors...,
	).WithParameters(
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 20, 1, 100),
		httpserver.QueryString("category"),
		httpserver.QueryString("result"),
	))
	rg.RegisterContract(http.MethodGet, "/admin/audit-logs/:id", httpserver.ResponseContract(
		response.Envelope[serviceaudit.AuditEventView]{}, http.StatusOK, errors...,
	))
}
