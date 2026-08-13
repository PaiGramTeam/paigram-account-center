package platformbinding

import (
	"context"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"google.golang.org/protobuf/proto"
)

type CredentialOperationReference struct {
	OperationID        string
	Kind               string
	BindingRef         string
	PreGeneration      uint64
	TargetGeneration   uint64
	RequestFingerprint string
}

type CredentialRemoteOperationState string

const (
	CredentialRemoteOperationPending             CredentialRemoteOperationState = "pending"
	CredentialRemoteOperationSucceeded           CredentialRemoteOperationState = "succeeded"
	CredentialRemoteOperationFailed              CredentialRemoteOperationState = "failed"
	CredentialRemoteOperationNotReceived         CredentialRemoteOperationState = "not_received"
	CredentialRemoteOperationFailedInputRequired CredentialRemoteOperationState = "failed_input_required"
)

type CredentialOperationResolution struct {
	State      CredentialRemoteOperationState
	ReasonCode string
	Summary    *RuntimeSummary
}

type CredentialBindingState struct {
	Exists  bool
	Summary *RuntimeSummary
}

type credentialOperationResolver interface {
	ResolveCredentialOperation(ctx context.Context, endpoint, ticket string, reference CredentialOperationReference) (*CredentialOperationResolution, error)
	GetCredentialBindingState(ctx context.Context, endpoint, ticket, bindingRef string) (*CredentialBindingState, error)
}

func (g *GRPCGenericCredentialGateway) ResolveCredentialOperation(ctx context.Context, endpoint, ticket string, reference CredentialOperationReference) (*CredentialOperationResolution, error) {
	operation, err := credentialOperationReferenceProto(reference)
	if err != nil {
		return nil, err
	}
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, reference.OperationID)
	defer cancel()
	response, err := platformv2.NewPlatformControlServiceClient(conn).ResolveOperation(callCtx, &platformv2.ResolveOperationRequest{Operation: operation})
	if err != nil {
		return nil, err
	}
	result := response.GetResult()
	if result == nil {
		return nil, errGenericCredentialSummaryRequired
	}
	if !proto.Equal(result.GetOperation(), operation) {
		return nil, errGenericCredentialOperationMismatch
	}

	resolution := &CredentialOperationResolution{ReasonCode: result.GetReasonCode()}
	switch result.GetState() {
	case platformv2.OperationState_OPERATION_STATE_PENDING:
		resolution.State = CredentialRemoteOperationPending
	case platformv2.OperationState_OPERATION_STATE_SUCCEEDED:
		resolution.State = CredentialRemoteOperationSucceeded
		summaryMap, mapErr := genericOperationResultMapForOperation(result, operation)
		if mapErr != nil {
			return nil, mapErr
		}
		resolution.Summary, err = decodeRuntimeSummary(summaryMap)
		if err != nil {
			return nil, err
		}
	case platformv2.OperationState_OPERATION_STATE_FAILED:
		resolution.State = CredentialRemoteOperationFailed
	case platformv2.OperationState_OPERATION_STATE_NOT_RECEIVED:
		resolution.State = CredentialRemoteOperationNotReceived
	case platformv2.OperationState_OPERATION_STATE_FAILED_INPUT_REQUIRED:
		resolution.State = CredentialRemoteOperationFailedInputRequired
	default:
		return nil, errGenericCredentialOperationFailed
	}
	return resolution, nil
}

func (g *GRPCGenericCredentialGateway) GetCredentialBindingState(ctx context.Context, endpoint, ticket, bindingRef string) (*CredentialBindingState, error) {
	if bindingRef == "" {
		return nil, ErrInvalidBindingMutation
	}
	conn, err := g.dial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, cancel := credentialGatewayCallContext(ctx, ticket, "")
	defer cancel()
	response, err := platformv2.NewPlatformControlServiceClient(conn).GetBindingState(callCtx, &platformv2.GetBindingStateRequest{BindingRef: bindingRef})
	if err != nil {
		return nil, err
	}
	state := response.GetState()
	if state == nil || state.GetBindingRef() != bindingRef {
		return nil, errGenericCredentialOperationMismatch
	}
	if !state.GetExists() {
		return &CredentialBindingState{Exists: false}, nil
	}
	return &CredentialBindingState{Exists: true, Summary: runtimeSummaryFromBindingState(state)}, nil
}

func credentialOperationReferenceProto(reference CredentialOperationReference) (*platformv2.OperationRef, error) {
	kindValue, ok := platformv2.OperationKind_value[reference.Kind]
	if !ok || reference.OperationID == "" || reference.BindingRef == "" || reference.RequestFingerprint == "" || reference.TargetGeneration != reference.PreGeneration+1 {
		return nil, ErrInvalidBindingMutation
	}
	return &platformv2.OperationRef{
		OperationId: reference.OperationID, Kind: platformv2.OperationKind(kindValue), BindingRef: reference.BindingRef,
		PreGeneration: reference.PreGeneration, TargetGeneration: reference.TargetGeneration, RequestFingerprint: reference.RequestFingerprint,
	}, nil
}

func runtimeSummaryFromBindingState(state *platformv2.BindingState) *RuntimeSummary {
	if state == nil {
		return nil
	}
	snapshot := state.GetProfileSnapshot()
	return &RuntimeSummary{
		PlatformAccountID: state.GetAccountKey(), Generation: state.GetCredentialGeneration(), Status: genericCredentialStatus(state.GetCredentialStatus()),
		LastValidatedAt: genericProtoTime(state.GetLastValidatedAt()), LastRefreshedAt: genericProtoTime(state.GetLastRefreshedAt()),
		Devices: []map[string]any{}, Profiles: genericProfileSummaries(snapshot.GetProfiles()),
		ProfileSnapshotComplete: snapshot.GetComplete(), ProfileRevision: snapshot.GetRevision(), ProfileObservedRevision: snapshot.GetObservedRevision(),
	}
}

var _ credentialOperationResolver = (*GRPCGenericCredentialGateway)(nil)
