package me

import (
	"net/http"

	authhandler "paigram/internal/handler/auth"
	mehandler "paigram/internal/handler/me"
	"paigram/internal/httpserver"
	"paigram/internal/response"
	serviceme "paigram/internal/service/me"
)

func registerContracts(rg *httpserver.Group) {
	readErrors := []int{http.StatusUnauthorized, http.StatusInternalServerError}
	rg.RegisterContract(http.MethodGet, "/me", httpserver.ResponseContract(
		response.Envelope[serviceme.CurrentUserView]{}, http.StatusOK, readErrors...,
	))
	rg.RegisterContract(http.MethodPatch, "/me", httpserver.JSONContract(
		mehandler.PatchMeRequest{}, response.Envelope[serviceme.CurrentUserView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/me/dashboard-summary", httpserver.ResponseContract(
		response.Envelope[serviceme.DashboardSummaryView]{}, http.StatusOK, readErrors...,
	))
	rg.RegisterContract(http.MethodGet, "/me/emails", httpserver.ResponseContract(
		response.Envelope[[]serviceme.EmailView]{}, http.StatusOK, readErrors...,
	))
	rg.RegisterContract(http.MethodPost, "/me/emails", httpserver.JSONContract(
		mehandler.CreateEmailRequest{}, response.Envelope[serviceme.CreatedEmailView]{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusInternalServerError,
	))
	messageResponse := response.Envelope[response.MessageData]{}
	for _, operation := range []struct {
		method string
		path   string
	}{
		{http.MethodDelete, "/me/emails/:emailId"},
		{http.MethodPatch, "/me/emails/:emailId/primary"},
		{http.MethodPost, "/me/emails/:emailId/verify"},
		{http.MethodPatch, "/me/login-methods/:provider/primary"},
		{http.MethodDelete, "/me/login-methods/:provider"},
	} {
		rg.RegisterContract(operation.method, operation.path, httpserver.ResponseContract(
			messageResponse, http.StatusOK,
			http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
		))
	}
	rg.RegisterContract(http.MethodPut, "/me/login-methods/:provider", httpserver.JSONContract(
		authhandler.InitiateOAuthRequest{}, authhandler.InitiateOAuthResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusNotFound, http.StatusInternalServerError,
	).WithOptionalBody())
	rg.RegisterContract(http.MethodGet, "/me/login-methods", httpserver.ResponseContract(
		response.Envelope[[]serviceme.LoginMethodView]{}, http.StatusOK, readErrors...,
	))
	rg.RegisterContract(http.MethodGet, "/me/security/overview", httpserver.ResponseContract(
		response.Envelope[serviceme.SecurityOverview]{}, http.StatusOK, readErrors...,
	))
	rg.RegisterContract(http.MethodPut, "/me/security/password", httpserver.JSONContract(
		mehandler.UpdatePasswordRequest{}, response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/me/security/2fa/setup", httpserver.JSONContract(
		mehandler.SetupTwoFactorRequest{}, response.Envelope[serviceme.TwoFactorSetupView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/me/security/2fa/confirm", httpserver.JSONContract(
		mehandler.ConfirmTwoFactorRequest{}, response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/me/security/2fa", httpserver.JSONContract(
		mehandler.DisableTwoFactorRequest{}, response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/me/security/2fa/backup-codes/regenerate", httpserver.JSONContract(
		mehandler.RegenerateBackupCodesRequest{}, response.Envelope[mehandler.MeBackupCodesResult]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/me/sessions/:sessionId", httpserver.ResponseContract(
		nil, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError,
	))
	pagination := []httpserver.Parameter{
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 20, 1, 100),
	}
	rg.RegisterContract(http.MethodGet, "/me/sessions", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[serviceme.SessionView]]{}, http.StatusOK, readErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodGet, "/me/activity-logs", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[serviceme.ActivityLogView]]{}, http.StatusOK, readErrors...,
	).WithParameters(append(pagination, httpserver.QueryString("action_type"))...))
}
