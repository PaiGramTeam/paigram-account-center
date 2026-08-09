package admin

import (
	"net/http"

	authorityhandler "paigram/internal/handler/authority"
	userhandler "paigram/internal/handler/user"
	"paigram/internal/httpserver"
	"paigram/internal/model"
	"paigram/internal/response"
)

func registerContracts(rg *httpserver.Group) {
	rg.RegisterContract(http.MethodPost, "/admin/users", httpserver.JSONContract(
		userhandler.CreateUserRequest{}, userhandler.CreateUserResponse{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError,
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
	))
	rg.RegisterContract(http.MethodPatch, "/admin/users/:id/status", httpserver.JSONContract(
		userhandler.UpdateUserStatusRequest{}, userhandler.UpdateUserStatusResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/users/:id/reset-password", httpserver.JSONContract(
		userhandler.ResetPasswordRequest{}, userhandler.ResetPasswordResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPut, "/admin/users/:id/roles", httpserver.JSONContract(
		userhandler.ReplaceUserRolesRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/admin/users/:id/primary-role", httpserver.JSONContract(
		userhandler.PatchPrimaryRoleRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/admin/users/:id/sessions/:sessionId", httpserver.ResponseContract(
		response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/roles", httpserver.JSONContract(
		authorityhandler.CreateAuthorityRequest{}, response.Envelope[model.Role]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError,
	))
	roleUpdate := httpserver.JSONContract(
		authorityhandler.UpdateAuthorityRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	)
	rg.RegisterContract(http.MethodPut, "/admin/roles/:id", roleUpdate)
	rg.RegisterContract(http.MethodPatch, "/admin/roles/:id", roleUpdate)
	rg.RegisterContract(http.MethodPut, "/admin/roles/:id/users", httpserver.JSONContract(
		authorityhandler.ReplaceAuthorityUsersRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPut, "/admin/roles/:id/permissions", httpserver.JSONContract(
		authorityhandler.AssignPermissionsRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/admin/roles/:id", httpserver.ResponseContract(
		response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
}
