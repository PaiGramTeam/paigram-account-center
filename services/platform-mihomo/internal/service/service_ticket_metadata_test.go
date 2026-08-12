package service

import (
	"context"
	"testing"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestPlatformRequestContractDoesNotExposeServiceTicket(t *testing.T) {
	messages := []protoreflect.MessageDescriptor{
		(&platformv1.GetCredentialSummaryRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.GetCredentialStatusRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.ValidateCredentialRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.ListProfilesRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.GetPrimaryProfileRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.ConfirmPrimaryProfileRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.GetAuthKeyRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.UpsertDeviceRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.PutCredentialRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.RefreshCredentialRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.DeleteCredentialRequest{}).ProtoReflect().Descriptor(),
		(&platformv1.InvalidateConsumerGrantRequest{}).ProtoReflect().Descriptor(),
	}
	for _, message := range messages {
		require.Nil(t, message.Fields().ByName("service_ticket"), string(message.FullName()))
	}
}

func TestGenericPlatformServiceAcceptsServiceTicketFromAuthorizationMetadata(t *testing.T) {
	store := newMemoryGrantInvalidationStore()
	adapter := newGenericPlatformServiceForAdapterTest(store)
	ticket := signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType: "admin",
		Scopes:    []string{"mihomo.consumer_grant.invalidate"},
	})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+ticket))

	resp, err := adapter.InvalidateConsumerGrant(ctx, &platformv1.InvalidateConsumerGrantRequest{
		BindingId:           101,
		Consumer:            "paimon-bot",
		MinimumGrantVersion: 3,
	})

	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
}

func TestServiceTicketMiddlewareVerifiesProtectedPlatformRPC(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	ticket := signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType:         "consumer",
		Consumer:          "paimon-bot",
		GrantVersion:      1,
		PlatformAccountID: "binding_101_10001",
		Scopes:            []string{"mihomo.credential.read_meta"},
	})
	ctx := incomingServiceTicketContext(ticket)
	ctx = transport.NewServerContext(ctx, adapterTestTransport{operation: platformv1.PlatformService_GetCredentialSummary_FullMethodName})

	called := false
	handler := adapter.ServiceTicketMiddleware()(func(ctx context.Context, _ any) (any, error) {
		called = true
		claims, ok := verifiedServiceTicketClaims(ctx)
		require.True(t, ok)
		require.Equal(t, "binding_101_10001", claims.PlatformAccountID)
		return struct{}{}, nil
	})

	_, err := handler(ctx, &platformv1.GetCredentialSummaryRequest{})
	require.NoError(t, err)
	require.True(t, called)
}

func TestServiceTicketMiddlewareRejectsMultipleActions(t *testing.T) {
	adapter := newGenericPlatformServiceForAdapterTest(newMemoryGrantInvalidationStore())
	ticket := signedAdapterServiceTicket(t, adapterTicketOptions{
		ActorType:         "consumer",
		Consumer:          "paimon-bot",
		GrantVersion:      1,
		PlatformAccountID: "binding_101_10001",
		Scopes:            []string{"mihomo.status.read", "mihomo.profile.read"},
	})
	ctx := incomingServiceTicketContext(ticket)
	ctx = transport.NewServerContext(ctx, adapterTestTransport{operation: platformv1.PlatformService_GetCredentialStatus_FullMethodName})
	called := false
	handler := adapter.ServiceTicketMiddleware()(func(context.Context, any) (any, error) {
		called = true
		return struct{}{}, nil
	})

	_, err := handler(ctx, &platformv1.GetCredentialStatusRequest{})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.False(t, called)
}

type adapterTestTransport struct {
	operation string
	header    adapterTestHeader
}

func (t adapterTestTransport) Kind() transport.Kind {
	return transport.KindGRPC
}

func (t adapterTestTransport) Endpoint() string {
	return "grpc://test"
}

func (t adapterTestTransport) Operation() string {
	return t.operation
}

func (t adapterTestTransport) RequestHeader() transport.Header {
	return t.header
}

func (t adapterTestTransport) ReplyHeader() transport.Header {
	return t.header
}

type adapterTestHeader map[string][]string

func (h adapterTestHeader) Get(key string) string {
	values := h[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (h adapterTestHeader) Set(key, value string) {
	h[key] = []string{value}
}

func (h adapterTestHeader) Add(key, value string) {
	h[key] = append(h[key], value)
}

func (h adapterTestHeader) Keys() []string {
	keys := make([]string, 0, len(h))
	for key := range h {
		keys = append(keys, key)
	}
	return keys
}

func (h adapterTestHeader) Values(key string) []string {
	return append([]string(nil), h[key]...)
}
