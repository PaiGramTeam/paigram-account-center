package adminsystem

import (
	"net/http"

	adminhandler "paigram/internal/handler/adminsystem"
	"paigram/internal/httpserver"
	"paigram/internal/response"
	"paigram/internal/service/botroute"
	serviceplatform "paigram/internal/service/platform"
	servicesystemconfig "paigram/internal/service/systemconfig"
)

type settingsPatch map[string]any
type legalDocumentsData struct {
	Documents []servicesystemconfig.LegalDocumentView `json:"documents"`
}

func registerContracts(rg *httpserver.Group) {
	settingsResponse := response.Envelope[servicesystemconfig.SettingsView]{}
	for _, path := range []string{
		"/admin/system/settings/site",
		"/admin/system/settings/registration",
		"/admin/system/settings/email",
		"/admin/system/auth-controls",
	} {
		rg.RegisterContract(http.MethodGet, path, httpserver.ResponseContract(
			settingsResponse, http.StatusOK,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
		))
		rg.RegisterContract(http.MethodPatch, path, httpserver.JSONContract(
			settingsPatch{}, settingsResponse, http.StatusOK,
			http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
		))
	}
	rg.RegisterContract(http.MethodGet, "/admin/system/settings/legal", httpserver.ResponseContract(
		response.Envelope[legalDocumentsData]{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/admin/system/settings/legal", httpserver.JSONContract(
		adminhandler.UpsertLegalDocumentsRequest{}, response.Envelope[legalDocumentsData]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/system/platform-services", httpserver.ResponseContract(
		response.Envelope[[]serviceplatform.PlatformServiceAdminView]{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/system/platform-services/:id", httpserver.ResponseContract(
		response.Envelope[serviceplatform.PlatformServiceAdminView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
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
	rg.RegisterContract(http.MethodGet, "/admin/system/bot-routes", httpserver.ResponseContract(
		response.Envelope[[]botroute.BotRouteAdminView]{}, http.StatusOK,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/admin/system/bot-routes/:id", httpserver.ResponseContract(
		response.Envelope[botroute.BotRouteAdminView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError,
	))
}
