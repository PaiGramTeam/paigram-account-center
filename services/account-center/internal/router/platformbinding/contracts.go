package platformbinding

import (
	"net/http"

	handlerplatformbinding "paigram/internal/handler/platformbinding"
	"paigram/internal/httpserver"
	"paigram/internal/response"
	serviceplatformbinding "paigram/internal/service/platformbinding"
)

type credentialPayload map[string]any

func registerContracts(rg *httpserver.Group) {
	meErrors := []int{
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict,
		http.StatusUnprocessableEntity, http.StatusServiceUnavailable, http.StatusInternalServerError,
	}
	adminErrors := append(append([]int(nil), meErrors...), http.StatusForbidden)
	pagination := []httpserver.Parameter{
		httpserver.QueryInteger("page", 1, 1, 0),
		httpserver.QueryInteger("page_size", 20, 1, 100),
	}
	rg.RegisterContract(http.MethodGet, "/me/platform-accounts", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[handlerplatformbinding.BindingView]]{}, http.StatusOK, meErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodPost, "/me/platform-accounts", httpserver.JSONContract(
		handlerplatformbinding.CreateBindingRequest{}, response.Envelope[handlerplatformbinding.BindingView]{}, http.StatusCreated,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	).WithErrorResponse(response.Envelope[handlerplatformbinding.CredentialOperationPendingView]{}, http.StatusAccepted))
	rg.RegisterContract(http.MethodGet, "/me/platform-accounts/:bindingId", httpserver.ResponseContract(
		response.Envelope[handlerplatformbinding.BindingView]{}, http.StatusOK, meErrors...,
	))
	rg.RegisterContract(http.MethodPatch, "/me/platform-accounts/:bindingId", httpserver.JSONContract(
		handlerplatformbinding.PatchBindingRequest{}, response.Envelope[handlerplatformbinding.BindingView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/me/platform-accounts/:bindingId/runtime-summary", httpserver.ResponseContract(
		response.Envelope[serviceplatformbinding.RuntimeSummary]{}, http.StatusOK, meErrors...,
	))
	rg.RegisterContract(http.MethodPatch, "/me/platform-accounts/:bindingId/primary-profile", httpserver.JSONContract(
		handlerplatformbinding.PatchPrimaryProfileRequest{}, response.Envelope[handlerplatformbinding.BindingView]{}, http.StatusOK,
		http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError,
	))
	rg.RegisterContract(http.MethodGet, "/me/platform-accounts/:bindingId/profiles", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[handlerplatformbinding.ProfileView]]{}, http.StatusOK, meErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodGet, "/me/platform-accounts/:bindingId/consumer-grants", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[handlerplatformbinding.ConsumerGrantView]]{}, http.StatusOK, meErrors...,
	).WithParameters(pagination...))
	registerBindingActions(rg, "/me/platform-accounts/:bindingId", false)

	rg.RegisterContract(http.MethodGet, "/admin/platform-accounts", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[handlerplatformbinding.AdminBindingView]]{}, http.StatusOK, adminErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodGet, "/admin/platform-accounts/:bindingId", httpserver.ResponseContract(
		response.Envelope[handlerplatformbinding.AdminBindingView]{}, http.StatusOK, adminErrors...,
	))
	rg.RegisterContract(http.MethodGet, "/admin/platform-accounts/:bindingId/profiles", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[handlerplatformbinding.ProfileView]]{}, http.StatusOK, adminErrors...,
	).WithParameters(pagination...))
	rg.RegisterContract(http.MethodGet, "/admin/platform-accounts/:bindingId/runtime-summary", httpserver.ResponseContract(
		response.Envelope[serviceplatformbinding.RuntimeSummary]{}, http.StatusOK, adminErrors...,
	))
	rg.RegisterContract(http.MethodGet, "/admin/platform-accounts/:bindingId/consumer-grants", httpserver.ResponseContract(
		response.Envelope[response.PaginatedData[handlerplatformbinding.ConsumerGrantView]]{}, http.StatusOK, adminErrors...,
	).WithParameters(pagination...))
	registerBindingActions(rg, "/admin/platform-accounts/:bindingId", true)
}

func registerBindingActions(rg *httpserver.Group, prefix string, admin bool) {
	commonErrors := []int{
		http.StatusBadRequest, http.StatusUnauthorized,
		http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError,
	}
	bindingResponse := any(response.Envelope[handlerplatformbinding.BindingView]{})
	if admin {
		commonErrors = append(commonErrors, http.StatusForbidden)
		bindingResponse = response.Envelope[handlerplatformbinding.AdminBindingView]{}
	}
	rg.RegisterContract(http.MethodPost, prefix+"/refresh", httpserver.ResponseContract(
		bindingResponse, http.StatusOK, commonErrors...,
	).WithErrorResponse(response.Envelope[handlerplatformbinding.CredentialOperationPendingView]{}, http.StatusAccepted))
	rg.RegisterContract(http.MethodPut, prefix+"/credential", httpserver.JSONContract(
		credentialPayload{}, response.Envelope[serviceplatformbinding.RuntimeSummary]{}, http.StatusOK, commonErrors...,
	).WithErrorResponse(response.Envelope[handlerplatformbinding.CredentialOperationPendingView]{}, http.StatusAccepted))
	rg.RegisterContract(http.MethodPut, prefix+"/consumer-grants/:consumer", httpserver.JSONContract(
		handlerplatformbinding.PutConsumerGrantRequest{}, response.Envelope[handlerplatformbinding.ConsumerGrantView]{}, http.StatusOK, commonErrors...,
	).WithErrorResponse(response.Envelope[handlerplatformbinding.GrantPropagationPendingView]{}, http.StatusAccepted))
	rg.RegisterContract(http.MethodDelete, prefix, httpserver.ResponseContract(
		nil, http.StatusNoContent, commonErrors...,
	).WithErrorResponse(response.Envelope[handlerplatformbinding.CredentialOperationPendingView]{}, http.StatusAccepted))
}
