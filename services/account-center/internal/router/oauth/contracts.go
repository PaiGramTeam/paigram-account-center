package oauth

import (
	"net/http"

	handleroauth "paigram/internal/handler/oauth"
	"paigram/internal/httpserver"
	"paigram/internal/response"
	"paigram/internal/service/credentials"
)

func registerAdminContracts(rg *httpserver.Group) {
	rg.RegisterContract(http.MethodGet, "/admin/service-credentials", httpserver.ResponseContract(
		response.Envelope[[]credentials.CredentialView]{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/service-credentials", httpserver.JSONContract(
		handleroauth.CreateRequest{}, response.Envelope[credentials.CreateResult]{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/service-credentials/:client_id/secret", httpserver.ResponseContract(
		response.Envelope[credentials.CreateResult]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
}
