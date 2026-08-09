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
	rg.RegisterContract(http.MethodPatch, "/me", httpserver.JSONContract(
		mehandler.PatchMeRequest{}, response.Envelope[serviceme.CurrentUserView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusInternalServerError,
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
		mehandler.RegenerateBackupCodesRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/me/sessions/:sessionId", httpserver.ResponseContract(
		nil, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError,
	))
}
