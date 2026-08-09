package platformbinding

import (
	"net/http"

	handlerplatformbinding "paigram/internal/handler/platformbinding"
	"paigram/internal/httpserver"
	"paigram/internal/response"
)

type credentialPayload map[string]any

func registerContracts(rg *httpserver.Group) {
	rg.RegisterContract(http.MethodPost, "/me/platform-accounts", httpserver.JSONContract(
		handlerplatformbinding.CreateBindingRequest{}, response.Envelope[map[string]any]{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/me/platform-accounts/:bindingId", httpserver.JSONContract(
		handlerplatformbinding.PatchBindingRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodPatch, "/me/platform-accounts/:bindingId/primary-profile", httpserver.JSONContract(
		handlerplatformbinding.PatchPrimaryProfileRequest{}, response.Envelope[map[string]any]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	registerBindingActions(rg, "/me/platform-accounts/:bindingId", http.StatusUnauthorized)
	registerBindingActions(rg, "/admin/platform-accounts/:bindingId", http.StatusForbidden)
}

func registerBindingActions(rg *httpserver.Group, prefix string, authorizationStatus int) {
	commonErrors := []int{
		http.StatusBadRequest, http.StatusUnauthorized,
		http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	}
	if authorizationStatus != http.StatusUnauthorized {
		commonErrors = append(commonErrors, authorizationStatus)
	}
	rg.RegisterContract(http.MethodPost, prefix+"/refresh", httpserver.ResponseContract(
		response.Envelope[map[string]any]{}, http.StatusOK, commonErrors...,
	))
	rg.RegisterContract(http.MethodPut, prefix+"/credential", httpserver.JSONContract(
		credentialPayload{}, response.Envelope[map[string]any]{}, http.StatusOK, commonErrors...,
	))
	rg.RegisterContract(http.MethodPut, prefix+"/consumer-grants/:consumer", httpserver.JSONContract(
		handlerplatformbinding.PutConsumerGrantRequest{}, response.Envelope[map[string]any]{}, http.StatusOK, commonErrors...,
	))
	rg.RegisterContract(http.MethodDelete, prefix, httpserver.ResponseContract(
		nil, http.StatusNoContent, commonErrors...,
	))
}
