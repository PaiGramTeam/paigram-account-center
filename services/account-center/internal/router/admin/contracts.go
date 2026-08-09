package admin

import (
	"net/http"

	authorityhandler "paigram/internal/handler/authority"
	userhandler "paigram/internal/handler/user"
	"paigram/internal/httpserver"
	"paigram/internal/model"
	"paigram/internal/response"
	serviceauthority "paigram/internal/service/authority"
	serviceme "paigram/internal/service/me"
)

type primaryRoleData struct {
	PrimaryRoleID any `json:"primary_role_id"`
}

func registerContracts(rg *httpserver.Group) {
	adminReadErrors := []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusInternalServerError,
	}
	pagination := []httpserver.Parameter{
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 20, 1, 100),
	}
	rg.RegisterContract(http.MethodGet, "/admin/users", httpserver.ResponseContract(
		userhandler.UserListResponse{}, http.StatusOK, adminReadErrors...,
	).WithParameters(
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 20, 1, 100),
		httpserver.QueryString("sort_by", "created_at", "last_login_at", "id"),
		httpserver.QueryString("order", "asc", "desc"),
		httpserver.QueryString("status", "active", "pending", "suspended", "deleted"),
		httpserver.QueryString("search"),
	))
	rg.RegisterContract(http.MethodPost, "/admin/users", httpserver.JSONContract(
		userhandler.CreateUserRequest{}, userhandler.CreateUserResponse{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id", httpserver.ResponseContract(
		userhandler.UserDetailResponse{}, http.StatusOK, adminReadErrors...,
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/login-methods", httpserver.ResponseContract(
		response.Envelope[[]serviceme.LoginMethodView]{}, http.StatusOK, adminReadErrors...,
	))
	rg.RegisterContract(http.MethodPatch, "/admin/users/:id", httpserver.JSONContract(
		userhandler.UpdateUserRequest{}, userhandler.UpdateUserResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/admin/users/:id/login-methods/:provider/primary", httpserver.ResponseContract(
		response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/admin/users/:id", httpserver.ResponseContract(
		nil, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	).WithParameters(httpserver.QueryBoolean("hard_delete", false)))
	rg.RegisterContract(http.MethodPatch, "/admin/users/:id/status", httpserver.JSONContract(
		userhandler.UpdateUserStatusRequest{}, userhandler.UpdateUserStatusResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/users/:id/reset-password", httpserver.JSONContract(
		userhandler.ResetPasswordRequest{}, userhandler.ResetPasswordResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/audit-logs", httpserver.ResponseContract(
		userhandler.AuditLogsResponse{}, http.StatusOK, adminReadErrors...,
	).WithParameters(append(pagination, httpserver.QueryString("action_type"))...))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/roles", httpserver.ResponseContract(
		userhandler.UserRolesResponse{}, http.StatusOK, adminReadErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodPut, "/admin/users/:id/roles", httpserver.JSONContract(
		userhandler.ReplaceUserRolesRequest{}, response.Envelope[primaryRoleData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/admin/users/:id/primary-role", httpserver.JSONContract(
		userhandler.PatchPrimaryRoleRequest{}, response.Envelope[primaryRoleData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/permissions", httpserver.ResponseContract(
		userhandler.UserPermissionsResponse{}, http.StatusOK, adminReadErrors...,
	).WithParameters(
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 50, 1, 100),
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/sessions", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[userhandler.SessionResponse]]{}, http.StatusOK, adminReadErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodDelete, "/admin/users/:id/sessions/:sessionId", httpserver.ResponseContract(
		response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/security-summary", httpserver.ResponseContract(
		response.Envelope[userhandler.SecuritySummary]{}, http.StatusOK, adminReadErrors...,
	))
	rg.RegisterContract(http.MethodGet, "/admin/users/:id/login-logs", httpserver.ResponseContract(
		userhandler.LoginLogsResponse{}, http.StatusOK, adminReadErrors...,
	).WithParameters(
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 20, 1, 100),
		httpserver.QueryString("status", "success", "failed"),
		httpserver.QueryDate("date_from"),
		httpserver.QueryDate("date_to"),
	))
	rg.RegisterContract(http.MethodPost, "/admin/roles", httpserver.JSONContract(
		authorityhandler.CreateAuthorityRequest{}, response.Envelope[model.Role]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/roles", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[serviceauthority.RoleWithPermissions]]{}, http.StatusOK, adminReadErrors...,
	).WithParameters(
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 10, 1, 0),
		httpserver.QueryString("name"),
	))
	rg.RegisterContract(http.MethodGet, "/admin/roles/:id", httpserver.ResponseContract(
		response.Envelope[model.Role]{}, http.StatusOK, adminReadErrors...,
	))
	roleUpdate := httpserver.JSONContract(
		authorityhandler.UpdateAuthorityRequest{}, response.Envelope[*struct{}]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	)
	rg.RegisterContract(http.MethodPut, "/admin/roles/:id", roleUpdate)
	rg.RegisterContract(http.MethodPatch, "/admin/roles/:id", roleUpdate)
	rg.RegisterContract(http.MethodPut, "/admin/roles/:id/users", httpserver.JSONContract(
		authorityhandler.ReplaceAuthorityUsersRequest{}, response.Envelope[*struct{}]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/roles/:id/users", httpserver.ResponseContract(
		response.Envelope[[]authorityhandler.AuthorityUserItem]{}, http.StatusOK, adminReadErrors...,
	))
	rg.RegisterContract(http.MethodPut, "/admin/roles/:id/permissions", httpserver.JSONContract(
		authorityhandler.AssignPermissionsRequest{}, response.Envelope[*struct{}]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/roles/:id/permissions", httpserver.ResponseContract(
		response.Envelope[[]model.Permission]{}, http.StatusOK, adminReadErrors...,
	))
	rg.RegisterContract(http.MethodDelete, "/admin/roles/:id", httpserver.ResponseContract(
		response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
}
