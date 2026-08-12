package service

import (
	"context"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/usecase"
)

func (s *GenericPlatformService) BindCredential(ctx context.Context, req *platformv1.BindCredentialRequest) (*platformv1.BindCredentialResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	claims, guard, err := s.authorizeCredentialMutation(ctx, usecase.ActionCredentialBind)
	if err != nil {
		return nil, err
	}
	bindInput, err := credentialBindInput(claims, req.GetCredentialPayloadJson())
	if err != nil {
		return nil, err
	}
	bound, err := s.bindUC.BindCredentialIfAbsent(ctx, bindInput)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	summary, err := s.managementUC.GetCredentialSummaryWithScope(ctx, guard, bound.PlatformAccountID)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.BindCredentialResponse{Summary: toGenericCredentialSummary(summary)}, nil
}

func (s *GenericPlatformService) ReplaceCredential(ctx context.Context, req *platformv1.ReplaceCredentialRequest) (*platformv1.ReplaceCredentialResponse, error) {
	if req == nil || req.GetPlatformAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "request and platform_account_id are required")
	}

	claims, guard, err := s.authorizeCredentialMutation(ctx, usecase.ActionCredentialUpdate)
	if err != nil {
		return nil, err
	}
	if err := guard.RequirePlatformAccountID(req.GetPlatformAccountId()); err != nil {
		return nil, mapUsecaseError(err)
	}
	bindInput, err := credentialBindInput(claims, req.GetCredentialPayloadJson())
	if err != nil {
		return nil, err
	}
	summary, err := s.managementUC.UpdateCredentialWithScope(ctx, guard, usecase.UpdateCredentialInput{
		PlatformAccountID:   req.GetPlatformAccountId(),
		BindCredentialInput: bindInput,
	})
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv1.ReplaceCredentialResponse{Summary: toGenericCredentialSummary(summary)}, nil
}

func (s *GenericPlatformService) authorizeCredentialMutation(ctx context.Context, action string) (*biz.ServiceTicketClaims, usecase.ScopeGuard, error) {
	claims, err := serviceTicketClaims(ctx, s.ticketVerifier)
	if err != nil {
		return nil, usecase.ScopeGuard{}, err
	}
	guard, err := scopedGuard(claims, action)
	if err != nil {
		return nil, usecase.ScopeGuard{}, mapUsecaseError(err)
	}
	if err := guard.RequireBindingWide(); err != nil {
		return nil, usecase.ScopeGuard{}, mapUsecaseError(err)
	}
	return claims, guard, nil
}

func credentialBindInput(claims *biz.ServiceTicketClaims, payloadJSON string) (usecase.BindCredentialInput, error) {
	payload, err := decodeGenericCredentialPayload(payloadJSON)
	if err != nil {
		return usecase.BindCredentialInput{}, status.Error(codes.InvalidArgument, "credential payload must be valid JSON")
	}
	return usecase.BindCredentialInput{
		BindingID:        claims.BindingID,
		CookieBundleJSON: payload.CookieBundle,
		DeviceID:         payload.DeviceID,
		DeviceFP:         payload.DeviceFP,
		DeviceName:       payload.DeviceName,
		RegionHint:       payload.RegionHint,
	}, nil
}
