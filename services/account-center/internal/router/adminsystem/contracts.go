package adminsystem

import (
	"net/http"

	adminhandler "paigram/internal/handler/adminsystem"
	"paigram/internal/httpserver"
	"paigram/internal/response"
	serviceplatform "paigram/internal/service/platform"
	servicesystemconfig "paigram/internal/service/systemconfig"
)

type settingsPatch map[string]any

func registerContracts(rg *httpserver.Group) {
	settingsResponse := response.Envelope[servicesystemconfig.SettingsView]{}
	for _, path := range []string{
		"/admin/system/settings/site",
		"/admin/system/settings/registration",
		"/admin/system/settings/email",
		"/admin/system/auth-controls",
	} {
		rg.RegisterContract(http.MethodPatch, path, httpserver.JSONContract(
			settingsPatch{}, settingsResponse, http.StatusOK,
			http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
		))
	}
	rg.RegisterContract(http.MethodPatch, "/admin/system/settings/legal", httpserver.JSONContract(
		adminhandler.UpsertLegalDocumentsRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/system/platform-services", httpserver.JSONContract(
		adminhandler.CreatePlatformServiceRequest{}, response.Envelope[serviceplatform.PlatformServiceAdminView]{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/admin/system/platform-services/:id", httpserver.JSONContract(
		adminhandler.UpdatePlatformServiceRequest{}, response.Envelope[serviceplatform.PlatformServiceAdminView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPost, "/admin/system/platform-services/:id/check", httpserver.ResponseContract(
		response.Envelope[serviceplatform.PlatformServiceAdminView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/admin/system/platform-services/:id", httpserver.ResponseContract(
		nil, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodDelete, "/admin/system/bot-routes/:id", httpserver.ResponseContract(
		nil, http.StatusNoContent,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
}
