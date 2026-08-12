package router

import (
	"net/http"

	authhandler "paigram/internal/handler/auth"
	"paigram/internal/httpserver"
	"paigram/internal/response"
)

func registerAuthContracts(auth *httpserver.Group, rateLimited bool) {
	limited := auth
	if rateLimited {
		limited = auth.WithErrorStatuses(http.StatusTooManyRequests)
	}
	limited.RegisterContract(http.MethodPost, "/register", httpserver.JSONContract(
		authhandler.RegisterEmailRequest{}, authhandler.RegisterEmailResponse{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusConflict, http.StatusTooManyRequests, http.StatusInternalServerError,
	))
	limited.RegisterContract(http.MethodPost, "/login", httpserver.JSONContract(
		authhandler.LoginEmailRequest{}, authhandler.LoginResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError,
	).WithSuccessResponseAlternatives(authhandler.LoginChallengeResponse{}))
	limited.RegisterContract(http.MethodPost, "/refresh", httpserver.ResponseContract(
		authhandler.LoginResponse{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	).WithSecuritySchemes("refreshCookie"))
	limited.RegisterContract(http.MethodPost, "/verify-email", httpserver.JSONContract(
		authhandler.VerifyEmailRequest{}, authhandler.VerifyEmailResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	limited.RegisterContract(http.MethodPost, "/forgot-password", httpserver.JSONContract(
		authhandler.ForgotPasswordRequest{}, response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusTooManyRequests, http.StatusInternalServerError,
	))
	limited.RegisterContract(http.MethodPost, "/reset-password", httpserver.JSONContract(
		authhandler.ResetPasswordRequest{}, response.Envelope[response.MessageData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError,
	))
	auth.RegisterContract(http.MethodPost, "/logout", httpserver.ResponseContract(
		authhandler.LogoutResponse{}, http.StatusOK,
		http.StatusForbidden, http.StatusInternalServerError,
	).WithSecuritySchemes("bearerAuth", "refreshCookie"))
	limited.RegisterContract(http.MethodPost, "/oauth/:provider/init", httpserver.JSONContract(
		authhandler.InitiateOAuthRequest{}, authhandler.InitiateOAuthResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError,
	).WithOptionalBody())
	limited.RegisterContract(http.MethodPost, "/oauth/:provider/callback", httpserver.JSONContract(
		authhandler.OAuthCallbackRequest{}, authhandler.OAuthCallbackResponse{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusBadGateway, http.StatusInternalServerError,
	))
}
