package platformbinding

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"paigram/internal/grpc/clientauth"
	"paigram/internal/model"
	"paigram/internal/platformtransport"
)

var errGenericCredentialSummaryRequired = errors.New("credential operation result is required")
var errGenericCredentialOperationFailed = errors.New("credential operation did not succeed")
var errGenericCredentialOperationMismatch = errors.New("credential operation result does not match request")

type CredentialGatewayDialFunc func(ctx context.Context, endpoint string) (*grpc.ClientConn, error)

type GRPCGenericCredentialGateway struct {
	dial CredentialGatewayDialFunc
}

func NewGRPCGenericCredentialGateway(dial func(context.Context, string) (*grpc.ClientConn, error)) *GRPCGenericCredentialGateway {
	if dial == nil {
		dial = func(context.Context, string) (*grpc.ClientConn, error) {
			return nil, platformtransport.ErrControlTransportNotConfigured
		}
	}
	return &GRPCGenericCredentialGateway{dial: CredentialGatewayDialFunc(dial)}
}

func (g *GRPCGenericCredentialGateway) BindCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding, payload json.RawMessage) (map[string]any, error) {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, operationID)
	defer cancel()
	operation := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, operationID)
	resp, err := platformv2.NewPlatformControlServiceClient(conn).BindCredential(callCtx, &platformv2.BindCredentialRequest{
		Operation:             operation,
		CredentialPayloadJson: string(payload),
	})
	if err != nil {
		return nil, err
	}
	return genericOperationResultMapForOperation(resp.GetResult(), operation)
}

func (g *GRPCGenericCredentialGateway) ReplaceCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding, payload json.RawMessage) (map[string]any, error) {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, operationID)
	defer cancel()
	operation := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, operationID)
	resp, err := platformv2.NewPlatformControlServiceClient(conn).ReplaceCredential(callCtx, &platformv2.ReplaceCredentialRequest{
		Operation:             operation,
		AccountKey:            bindingExternalAccountKey(binding),
		CredentialPayloadJson: string(payload),
	})
	if err != nil {
		return nil, err
	}
	return genericOperationResultMapForOperation(resp.GetResult(), operation)
}

func (g *GRPCGenericCredentialGateway) RefreshCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding) (*RuntimeSummary, error) {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, operationID)
	defer cancel()
	operation := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_REFRESH_CREDENTIAL, operationID)
	resp, err := platformv2.NewPlatformControlServiceClient(conn).RefreshCredential(callCtx, &platformv2.RefreshCredentialRequest{
		Operation:  operation,
		AccountKey: bindingExternalAccountKey(binding),
	})
	if err != nil {
		return nil, err
	}
	summary, err := genericOperationResultMapForOperation(resp.GetResult(), operation)
	if err != nil {
		return nil, err
	}
	return decodeRuntimeSummary(summary)
}

func (g *GRPCGenericCredentialGateway) DeleteCredential(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding) error {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, operationID)
	defer cancel()
	operation := newOperationRef(binding, platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL, operationID)
	resp, err := platformv2.NewPlatformControlServiceClient(conn).DeleteCredential(callCtx, &platformv2.DeleteCredentialRequest{
		Operation:  operation,
		AccountKey: bindingExternalAccountKey(binding),
	})
	if err != nil {
		return err
	}
	return requireSucceededOperationResult(resp.GetResult(), operation)
}

func (g *GRPCGenericCredentialGateway) SetPrimaryProfile(ctx context.Context, endpoint, ticket, operationID string, binding *model.PlatformAccountBinding, profileRef string) (*RuntimeSummary, error) {
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, operationID)
	defer cancel()
	operation := newPrimaryProfileOperationRef(binding, operationID, profileRef)
	resp, err := platformv2.NewPlatformControlServiceClient(conn).SetPrimaryProfile(callCtx, &platformv2.SetPrimaryProfileRequest{
		Operation:               operation,
		AccountKey:              bindingExternalAccountKey(binding),
		ProfileRef:              profileRef,
		ExpectedProfileRevision: binding.ProfileRevision,
	})
	if err != nil {
		return nil, err
	}
	summary, err := genericOperationResultMapForOperation(resp.GetResult(), operation)
	if err != nil {
		return nil, err
	}
	return decodeRuntimeSummary(summary)
}

func newOperationRef(binding *model.PlatformAccountBinding, kind platformv2.OperationKind, operationID string) *platformv2.OperationRef {
	if binding == nil {
		return nil
	}
	fingerprint := operationid.Fingerprint(kind.String(), binding.BindingRef, binding.Generation, binding.Generation+1)
	return &platformv2.OperationRef{
		OperationId:        operationID,
		Kind:               kind,
		BindingRef:         binding.BindingRef,
		PreGeneration:      binding.Generation,
		TargetGeneration:   binding.Generation + 1,
		RequestFingerprint: fingerprint,
	}
}

func newPrimaryProfileOperationRef(binding *model.PlatformAccountBinding, operationID, profileRef string) *platformv2.OperationRef {
	if binding == nil {
		return nil
	}
	kind := platformv2.OperationKind_OPERATION_KIND_SET_PRIMARY_PROFILE
	return &platformv2.OperationRef{
		OperationId:        operationID,
		Kind:               kind,
		BindingRef:         binding.BindingRef,
		PreGeneration:      binding.Generation,
		TargetGeneration:   binding.Generation,
		RequestFingerprint: operationid.PrimaryProfileFingerprint(kind.String(), binding.BindingRef, profileRef, binding.Generation, binding.ProfileRevision),
	}
}

func credentialGatewayCallContext(ctx context.Context, ticket, operationID string) (context.Context, context.CancelFunc) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	callCtx = correlation.WithOperationID(callCtx, operationID)
	return clientauth.WithServiceTicket(callCtx, ticket), cancel
}

func bindingExternalAccountKey(binding *model.PlatformAccountBinding) string {
	if binding == nil || !binding.ExternalAccountKey.Valid {
		return ""
	}
	return binding.ExternalAccountKey.String
}

func genericOperationResultMap(result *platformv2.OperationResult) (map[string]any, error) {
	return genericOperationResultMapForOperation(result, nil)
}

func genericOperationResultMapForOperation(result *platformv2.OperationResult, expected *platformv2.OperationRef) (map[string]any, error) {
	if err := requireSucceededOperationResult(result, expected); err != nil {
		return nil, err
	}
	return map[string]any{
		"platform_account_id":       result.GetAccountKey(),
		"generation":                result.GetOperation().GetTargetGeneration(),
		"status":                    genericCredentialStatus(result.GetCredentialStatus()),
		"last_validated_at":         genericProtoTime(result.GetLastValidatedAt()),
		"last_refreshed_at":         genericProtoTime(result.GetLastRefreshedAt()),
		"devices":                   []map[string]any{},
		"profiles":                  genericProfileSummaries(result.GetProfileSnapshot().GetProfiles()),
		"profile_snapshot_complete": result.GetProfileSnapshot().GetComplete(),
		"profile_revision":          result.GetProfileSnapshot().GetRevision(),
		"profile_observed_revision": result.GetProfileSnapshot().GetObservedRevision(),
	}, nil
}

func requireSucceededOperationResult(result *platformv2.OperationResult, expected *platformv2.OperationRef) error {
	if result == nil {
		return errGenericCredentialSummaryRequired
	}
	if result.GetState() != platformv2.OperationState_OPERATION_STATE_SUCCEEDED {
		return errGenericCredentialOperationFailed
	}
	if expected != nil && !proto.Equal(result.GetOperation(), expected) {
		return errGenericCredentialOperationMismatch
	}
	return nil
}

func genericCredentialStatus(status platformv2.CredentialStatus) string {
	switch status {
	case platformv2.CredentialStatus_CREDENTIAL_STATUS_ACTIVE:
		return "active"
	case platformv2.CredentialStatus_CREDENTIAL_STATUS_EXPIRED:
		return "expired"
	case platformv2.CredentialStatus_CREDENTIAL_STATUS_INVALID:
		return "invalid"
	case platformv2.CredentialStatus_CREDENTIAL_STATUS_CHALLENGE_REQUIRED:
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

func genericProfileSummaries(profiles []*platformv2.ProfileSummary) []map[string]any {
	items := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, map[string]any{
			"profile_ref": profile.GetProfileRef(), "platform_account_id": profile.GetAccountKey(), "game_biz": profile.GetGameBiz(),
			"region": profile.GetRegion(), "player_id": profile.GetPlayerId(), "nickname": profile.GetNickname(),
			"level": profile.GetLevel(), "is_default": profile.GetIsDefault(),
		})
	}
	return items
}
