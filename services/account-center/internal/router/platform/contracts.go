package platform

import (
	"net/http"

	"paigram/internal/httpserver"
	"paigram/internal/response"
	serviceplatform "paigram/internal/service/platform"
)

func registerContracts(rg *httpserver.Group) {
	rg.RegisterContract(http.MethodGet, "/me/platforms", httpserver.ResponseContract(
		response.Envelope[[]serviceplatform.PlatformListView]{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/me/platforms/:platform/schema", httpserver.ResponseContract(
		response.Envelope[serviceplatform.PlatformSchemaView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError,
	))
}
