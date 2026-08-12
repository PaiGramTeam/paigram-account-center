package service

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
	"platform-mihomo-service/internal/usecase"
)

const serviceTicketAudience = "platform-mihomo-service"

func mapTicketVerificationError(err error) error {
	if errors.Is(err, data.ErrGrantVersionRevoked) {
		return status.Error(codes.PermissionDenied, "service ticket grant has been revoked")
	}
	return status.Error(codes.Unauthenticated, "invalid service ticket")
}

func mapUsecaseError(err error) error {
	if errors.Is(err, biz.ErrCredentialAlreadyBound) {
		return status.Error(codes.AlreadyExists, "credential already exists for binding")
	}
	if errors.Is(err, usecase.ErrCredentialNotFound) {
		return status.Error(codes.NotFound, "credential not found")
	}
	if errors.Is(err, usecase.ErrProfileNotFound) {
		return status.Error(codes.NotFound, "profile not found")
	}
	if errors.Is(err, usecase.ErrPlatformAccountMismatch) {
		return status.Error(codes.PermissionDenied, "platform account is outside ticket scope")
	}
	if errors.Is(err, usecase.ErrActionScopeDenied) || errors.Is(err, usecase.ErrBindingScopeDenied) || errors.Is(err, usecase.ErrProfileScopeDenied) {
		return status.Error(codes.PermissionDenied, "request is outside ticket scope")
	}
	var upstreamErr *platformmihomo.UpstreamError
	if errors.As(err, &upstreamErr) {
		switch upstreamErr.Kind {
		case platformmihomo.ErrorRateLimited:
			return status.Error(codes.ResourceExhausted, "platform upstream rate limit exceeded")
		case platformmihomo.ErrorUnavailable, platformmihomo.ErrorInvalidResponse:
			return status.Error(codes.Unavailable, "platform upstream is unavailable")
		case platformmihomo.ErrorInvalidCredential, platformmihomo.ErrorExpiredCredential, platformmihomo.ErrorChallengeRequired:
			return status.Error(codes.FailedPrecondition, "platform credential requires user attention")
		}
	}

	return status.Error(codes.Internal, "platform operation failed")
}

func scopedGuardForPlatformAccount(claims *biz.ServiceTicketClaims, platformAccountID string, requiredActions ...string) (usecase.ScopeGuard, error) {
	guard, err := scopedGuard(claims, requiredActions...)
	if err != nil {
		return usecase.ScopeGuard{}, err
	}
	if err := guard.RequirePlatformAccountID(platformAccountID); err != nil {
		return usecase.ScopeGuard{}, err
	}
	return guard, nil
}

func scopedGuard(claims *biz.ServiceTicketClaims, requiredActions ...string) (usecase.ScopeGuard, error) {
	guard, err := toScopeGuard(claims)
	if err != nil {
		return usecase.ScopeGuard{}, err
	}
	for _, action := range requiredActions {
		if err := guard.RequireAction(action); err != nil {
			return usecase.ScopeGuard{}, err
		}
	}
	return guard, nil
}

func toScopeGuard(claims *biz.ServiceTicketClaims) (usecase.ScopeGuard, error) {
	if claims == nil || claims.Platform != "mihomo" {
		return usecase.ScopeGuard{}, usecase.ErrBindingScopeDenied
	}

	allowedActions := make(map[string]struct{}, len(claims.Scopes))
	for _, scope := range claims.Scopes {
		allowedActions[scope] = struct{}{}
	}

	return usecase.ScopeGuard{
		AllowedActions: allowedActions,
		BindingID:      claims.BindingID,
		ProfileID:      claims.ProfileID,
	}, nil
}
