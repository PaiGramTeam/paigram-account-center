package service

import (
	"context"
	"strings"

	mihomov2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/mihomo/v2"
	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/observability"
)

var controlServiceOperationPrefix = "/" + platformv2.PlatformControlService_ServiceDesc.ServiceName + "/"
var runtimeServiceOperationPrefix = "/" + mihomov2.MihomoRuntimeService_ServiceDesc.ServiceName + "/"

type verifiedServiceTicketClaimsKey struct{}

type ticketRejectionRecorder interface {
	RecordTicketRejection(surface, reason string)
}

// ServiceTicketMiddleware verifies tickets for protected v2 platform RPCs before dispatch.
func (s *PlatformControlService) ServiceTicketMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			serverTransport, ok := transport.FromServerContext(ctx)
			if !ok || !isProtectedPlatformOperation(serverTransport.Operation()) {
				return next(ctx, req)
			}
			claims, err := verifyIncomingServiceTicket(ctx, s.ticketVerifier)
			if err != nil {
				s.recordTicketRejection(serverTransport.Operation(), err)
				return nil, err
			}
			response, err := next(contextWithVerifiedServiceTicketClaims(ctx, claims), req)
			if code := status.Code(err); code == codes.Unauthenticated || code == codes.PermissionDenied {
				s.recordTicketRejection(serverTransport.Operation(), err)
			}
			return response, err
		}
	}
}

func (s *PlatformControlService) recordTicketRejection(operation string, err error) {
	if s == nil || s.ticketMetrics == nil {
		return
	}
	s.ticketMetrics.RecordTicketRejection(ticketSurface(operation), status.Code(err).String())
}

func ticketSurface(operation string) string {
	if strings.HasPrefix(operation, controlServiceOperationPrefix) {
		return "control"
	}
	if strings.HasPrefix(operation, runtimeServiceOperationPrefix) {
		return "runtime"
	}
	return "other"
}

func contextWithVerifiedServiceTicketClaims(ctx context.Context, claims *biz.ServiceTicketClaims) context.Context {
	ctx = context.WithValue(ctx, verifiedServiceTicketClaimsKey{}, claims)
	if claims == nil {
		return observability.WithVerifiedOperation(ctx, "")
	}
	return observability.WithVerifiedOperation(ctx, claims.OperationID)
}

func isProtectedPlatformOperation(operation string) bool {
	if strings.HasPrefix(operation, controlServiceOperationPrefix) {
		return true
	}
	return strings.HasPrefix(operation, runtimeServiceOperationPrefix) && operation != mihomov2.MihomoRuntimeService_DescribePlatform_FullMethodName
}

func serviceTicketClaims(ctx context.Context, verifier *data.TicketVerifier) (*biz.ServiceTicketClaims, error) {
	if claims, ok := verifiedServiceTicketClaims(ctx); ok {
		return claims, nil
	}
	return verifyIncomingServiceTicket(ctx, verifier)
}

func verifiedServiceTicketClaims(ctx context.Context) (*biz.ServiceTicketClaims, bool) {
	claims, ok := ctx.Value(verifiedServiceTicketClaimsKey{}).(*biz.ServiceTicketClaims)
	return claims, ok && claims != nil
}

func verifyIncomingServiceTicket(ctx context.Context, verifier *data.TicketVerifier) (*biz.ServiceTicketClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "service ticket metadata is required")
	}
	values := md.Get("authorization")
	if len(values) != 1 {
		return nil, status.Error(codes.Unauthenticated, "exactly one authorization value is required")
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization metadata")
	}
	claims, err := verifier.VerifyContext(ctx, parts[1], serviceTicketAudience)
	if err != nil {
		return nil, mapTicketVerificationError(err)
	}
	if len(claims.Scopes) != 1 {
		return nil, status.Error(codes.PermissionDenied, "service ticket must grant exactly one action")
	}
	return claims, nil
}
