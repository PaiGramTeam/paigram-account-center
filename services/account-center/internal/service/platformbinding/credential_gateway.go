package platformbinding

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"paigram/internal/grpc/clientauth"
	"paigram/internal/model"
)

var errGenericCredentialSummaryRequired = errors.New("credential summary is required")

type CredentialGatewayDialFunc func(ctx context.Context, endpoint string) (*grpc.ClientConn, error)

type GRPCGenericCredentialGateway struct {
	dial CredentialGatewayDialFunc
}

func NewGRPCGenericCredentialGateway(dial CredentialGatewayDialFunc) *GRPCGenericCredentialGateway {
	if dial == nil {
		dial = dialGenericPlatform
	}
	return &GRPCGenericCredentialGateway{dial: dial}
}

func (g *GRPCGenericCredentialGateway) BindCredential(ctx context.Context, endpoint, ticket string, _ *model.PlatformAccountBinding, payload json.RawMessage) (map[string]any, error) {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket)
	defer cancel()
	resp, err := platformv1.NewPlatformServiceClient(conn).BindCredential(callCtx, &platformv1.BindCredentialRequest{CredentialPayloadJson: string(payload)})
	if err != nil {
		return nil, err
	}
	return genericCredentialSummaryMap(resp.GetSummary())
}

func (g *GRPCGenericCredentialGateway) ReplaceCredential(ctx context.Context, endpoint, ticket string, binding *model.PlatformAccountBinding, payload json.RawMessage) (map[string]any, error) {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket)
	defer cancel()
	resp, err := platformv1.NewPlatformServiceClient(conn).ReplaceCredential(callCtx, &platformv1.ReplaceCredentialRequest{
		PlatformAccountId:     bindingExternalAccountKey(binding),
		CredentialPayloadJson: string(payload),
	})
	if err != nil {
		return nil, err
	}
	return genericCredentialSummaryMap(resp.GetSummary())
}

func (g *GRPCGenericCredentialGateway) RefreshCredential(ctx context.Context, endpoint, ticket string, binding *model.PlatformAccountBinding) error {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket)
	defer cancel()
	_, err = platformv1.NewPlatformServiceClient(conn).RefreshCredential(callCtx, &platformv1.RefreshCredentialRequest{
		PlatformAccountId: bindingExternalAccountKey(binding),
	})
	return err
}

func (g *GRPCGenericCredentialGateway) DeleteCredential(ctx context.Context, endpoint, ticket string, binding *model.PlatformAccountBinding) error {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket)
	defer cancel()
	_, err = platformv1.NewPlatformServiceClient(conn).DeleteCredential(callCtx, &platformv1.DeleteCredentialRequest{
		PlatformAccountId: bindingExternalAccountKey(binding),
	})
	return err
}

func credentialGatewayCallContext(ctx context.Context, ticket string) (context.Context, context.CancelFunc) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	return clientauth.WithServiceTicket(callCtx, ticket), cancel
}

func dialGenericPlatform(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return grpc.DialContext(dialCtx, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
}

func bindingExternalAccountKey(binding *model.PlatformAccountBinding) string {
	if binding == nil || !binding.ExternalAccountKey.Valid {
		return ""
	}
	return binding.ExternalAccountKey.String
}

func genericCredentialSummaryMap(resp *platformv1.GetCredentialSummaryResponse) (map[string]any, error) {
	if resp == nil {
		return nil, errGenericCredentialSummaryRequired
	}
	return map[string]any{
		"platform_account_id": resp.GetPlatformAccountId(),
		"status":              genericCredentialStatus(resp.GetStatus()),
		"last_validated_at":   genericProtoTime(resp.GetLastValidatedAt()),
		"last_refreshed_at":   genericProtoTime(resp.GetLastRefreshedAt()),
		"devices":             genericDeviceSummaries(resp.GetDevices()),
		"profiles":            genericProfileSummaries(resp.GetProfiles()),
	}, nil
}

func genericCredentialStatus(status platformv1.CredentialStatus) string {
	switch status {
	case platformv1.CredentialStatus_CREDENTIAL_STATUS_ACTIVE:
		return "active"
	case platformv1.CredentialStatus_CREDENTIAL_STATUS_EXPIRED:
		return "expired"
	case platformv1.CredentialStatus_CREDENTIAL_STATUS_INVALID:
		return "invalid"
	case platformv1.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED:
		return "challenge_required"
	default:
		return "unspecified"
	}
}

func genericProtoTime(value *timestamppb.Timestamp) any {
	if value == nil {
		return nil
	}
	return value.AsTime().UTC().Format(time.RFC3339)
}

func genericDeviceSummaries(devices []*platformv1.DeviceSummary) []map[string]any {
	items := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		items = append(items, map[string]any{
			"device_id": device.GetDeviceId(), "device_fp": device.GetDeviceFp(), "device_name": device.GetDeviceName(),
			"is_valid": device.GetIsValid(), "last_seen_at": genericProtoTime(device.GetLastSeenAt()),
		})
	}
	return items
}

func genericProfileSummaries(profiles []*platformv1.ProfileSummary) []map[string]any {
	items := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, map[string]any{
			"id": profile.GetId(), "platform_account_id": profile.GetPlatformAccountId(), "game_biz": profile.GetGameBiz(),
			"region": profile.GetRegion(), "player_id": profile.GetPlayerId(), "nickname": profile.GetNickname(),
			"level": profile.GetLevel(), "is_default": profile.GetIsDefault(),
		})
	}
	return items
}
