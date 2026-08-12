package platform

import (
	"context"
	"time"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"google.golang.org/grpc"

	"paigram/internal/grpc/clientauth"
	"paigram/internal/platformtransport"
)

type GRPCGenericSummaryProxy struct {
	dial dialFunc
}

func NewGRPCGenericSummaryProxy(dial func(context.Context, string) (*grpc.ClientConn, error)) *GRPCGenericSummaryProxy {
	if dial == nil {
		dial = func(context.Context, string) (*grpc.ClientConn, error) {
			return nil, platformtransport.ErrControlTransportNotConfigured
		}
	}
	return &GRPCGenericSummaryProxy{dial: dialFunc(dial)}
}

func (p *GRPCGenericSummaryProxy) GetCredentialSummary(ctx context.Context, endpoint, ticket, bindingRef, accountKey string) (map[string]any, error) {
	conn, err := p.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	callCtx = clientauth.WithServiceTicket(callCtx, ticket)
	resp, err := platformv2.NewPlatformControlServiceClient(conn).GetBindingState(callCtx, &platformv2.GetBindingStateRequest{BindingRef: bindingRef})
	if err != nil {
		return nil, err
	}
	state := resp.GetState()
	if state == nil || !state.GetExists() || state.GetBindingRef() != bindingRef || state.GetAccountKey() != accountKey {
		return nil, ErrPlatformServiceUnavailable
	}
	return map[string]any{
		"platform_account_id":       state.GetAccountKey(),
		"generation":                state.GetCredentialGeneration(),
		"status":                    mapGenericCredentialStatus(state.GetCredentialStatus()),
		"last_validated_at":         formatProtoTime(state.GetLastValidatedAt()),
		"last_refreshed_at":         formatProtoTime(state.GetLastRefreshedAt()),
		"devices":                   []map[string]any{},
		"profiles":                  buildGenericProfileSummaries(state.GetProfileSnapshot().GetProfiles()),
		"profile_snapshot_complete": state.GetProfileSnapshot().GetComplete(),
		"profile_revision":          state.GetProfileSnapshot().GetRevision(),
		"profile_observed_revision": state.GetProfileSnapshot().GetObservedRevision(),
	}, nil
}

func buildGenericProfileSummaries(profiles []*platformv2.ProfileSummary) []map[string]any {
	views := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		views = append(views, map[string]any{
			"profile_ref": profile.GetProfileRef(), "platform_account_id": profile.GetAccountKey(),
			"game_biz": profile.GetGameBiz(), "region": profile.GetRegion(), "player_id": profile.GetPlayerId(),
			"nickname": profile.GetNickname(), "level": profile.GetLevel(), "is_default": profile.GetIsDefault(),
		})
	}
	return views
}

func mapGenericCredentialStatus(status platformv2.CredentialStatus) string {
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
