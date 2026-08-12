package interceptor

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"paigram/internal/service/credentials"
)

// machineTokenAudience is the JWT aud claim every protected gRPC method
// requires (matches the audience the SDK + telegram OAuth clients
// request from /oauth/token).
const machineTokenAudience = "account-center"

// credentialClaimsContextKey is the context key under which we store the
// validated AccessClaims for handler-side scope checks. Defined as an
// empty struct so it cannot collide with string-keyed context values.
type credentialClaimsContextKey struct{}

// AuthInterceptor verifies HS256 OAuth 2.0 access tokens on every
// inbound gRPC unary and stream call. Validation includes a credential
// registry lookup so disabled clients are rejected immediately.
type AuthInterceptor struct {
	tokens *credentials.TokenService

	// publicMethods is the explicit allowlist of fully-qualified method
	// names that skip authentication. After dropping BotAuthService the
	// set is empty by design; we keep the map structure so future
	// public RPCs (e.g. health checks) can be added without changing
	// the call path.
	publicMethods map[string]bool
}

// NewAuthInterceptor wires the interceptor against the OAuth token service.
func NewAuthInterceptor(tokens *credentials.TokenService) *AuthInterceptor {
	return &AuthInterceptor{
		tokens: tokens,
		publicMethods: map[string]bool{
			healthpb.Health_Check_FullMethodName: true,
			healthpb.Health_List_FullMethodName:  true,
			healthpb.Health_Watch_FullMethodName: true,
		},
	}
}

// Unary returns the unary server interceptor that authenticates every
// non-public call.
func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if i.publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}
		ctx, err := i.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream returns the stream server interceptor counterpart of Unary.
func (i *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if i.publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}
		ctx, err := i.authenticate(ss.Context())
		if err != nil {
			return err
		}
		return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: ctx})
	}
}

// authenticate extracts the Bearer token, validates it against the
// shared HS256 key + the audience constant, and attaches the parsed
// claims to the returned context under credentialClaimsContextKey{}.
func (i *AuthInterceptor) authenticate(ctx context.Context) (context.Context, error) {
	token, err := extractBearerToken(ctx)
	if err != nil {
		return ctx, err
	}
	claims, err := i.tokens.ValidateAccessToken(token, machineTokenAudience)
	if err != nil {
		switch {
		case errors.Is(err, credentials.ErrCredentialNotFound),
			errors.Is(err, credentials.ErrCredentialDisabled):
			return ctx, status.Error(codes.Unauthenticated, "credential is not active")
		default:
			return ctx, status.Error(codes.Unauthenticated, "invalid access token")
		}
	}
	return context.WithValue(ctx, credentialClaimsContextKey{}, claims), nil
}

// CredentialClaimsFromContext returns the validated AccessClaims attached
// by AuthInterceptor.authenticate. Callers in the service layer use this
// to read client_id (for ownership / audit) and the scope set (for
// authorization).
func CredentialClaimsFromContext(ctx context.Context) (*credentials.AccessClaims, bool) {
	claims, ok := ctx.Value(credentialClaimsContextKey{}).(*credentials.AccessClaims)
	return claims, ok
}

// WithCredentialClaims attaches a validated AccessClaims pointer to ctx
// under the same private key that AuthInterceptor uses. Exported so
// service-layer tests can synthesise an authenticated context without
// spinning up the real interceptor + token-service pipeline.
func WithCredentialClaims(ctx context.Context, claims *credentials.AccessClaims) context.Context {
	return context.WithValue(ctx, credentialClaimsContextKey{}, claims)
}

// CheckScope returns true iff the caller's access token carries the
// required scope (or the wildcard admin.all).
func CheckScope(ctx context.Context, requiredScope string) bool {
	claims, ok := CredentialClaimsFromContext(ctx)
	if !ok {
		return false
	}
	return credentials.HasScope(claims, requiredScope)
}

// extractBearerToken reads "authorization: Bearer <jwt>" from the
// incoming metadata. Returns Unauthenticated on any structural problem.
func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}
	authorization := md.Get("authorization")
	if len(authorization) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}
	parts := strings.SplitN(authorization[0], " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", status.Error(codes.Unauthenticated, "invalid authorization header format")
	}
	return parts[1], nil
}

// wrappedServerStream overrides ServerStream.Context() so handler code
// sees the claims-augmented context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
