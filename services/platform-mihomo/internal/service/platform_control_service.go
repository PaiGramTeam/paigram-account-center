package service

import (
	"context"
	"encoding/json"
	"errors"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/operationid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"platform-mihomo-service/internal/biz"
	"platform-mihomo-service/internal/data"
	"platform-mihomo-service/internal/usecase"
)

type grantInvalidationStore interface {
	Upsert(ctx context.Context, bindingRef string, consumer string, minimumVersion uint64) error
}

type PlatformControlService struct {
	platformv2.UnimplementedPlatformControlServiceServer

	ticketVerifier   *data.TicketVerifier
	operationUC      *usecase.OperationUsecase
	bindUC           *usecase.BindUsecase
	statusUC         *usecase.StatusUsecase
	managementUC     *usecase.ManagementUsecase
	credentials      biz.CredentialRepository
	fences           biz.AuthorizationFenceRepository
	invalidationRepo grantInvalidationStore
}

func NewPlatformControlService(
	ticketVerifier *data.TicketVerifier,
	operationUC *usecase.OperationUsecase,
	bindUC *usecase.BindUsecase,
	statusUC *usecase.StatusUsecase,
	managementUC *usecase.ManagementUsecase,
	credentials biz.CredentialRepository,
	fences biz.AuthorizationFenceRepository,
	invalidationRepo grantInvalidationStore,
) *PlatformControlService {
	return &PlatformControlService{
		ticketVerifier:   ticketVerifier,
		operationUC:      operationUC,
		bindUC:           bindUC,
		statusUC:         statusUC,
		managementUC:     managementUC,
		credentials:      credentials,
		fences:           fences,
		invalidationRepo: invalidationRepo,
	}
}

func (s *PlatformControlService) BindCredential(ctx context.Context, req *platformv2.BindCredentialRequest) (*platformv2.BindCredentialResponse, error) {
	requestOperation := operationRequest(req)
	operation, claims, err := s.authorizeOperation(ctx, requestOperation, platformv2.OperationKind_OPERATION_KIND_BIND_CREDENTIAL, usecase.ActionCredentialBind, "", operationFingerprint(requestOperation))
	if err != nil {
		return nil, err
	}
	payload, err := decodeCredentialPayload(req.GetCredentialPayloadJson())
	if err != nil {
		return nil, err
	}
	result, err := s.operationUC.Execute(ctx, operation, func(txCtx context.Context) (*biz.OperationResult, error) {
		if operation.PreGeneration != 0 || operation.TargetGeneration != 1 {
			return nil, status.Error(codes.InvalidArgument, "bind operation must transition generation 0 to 1")
		}
		bound, err := s.bindUC.BindCredentialIfAbsent(txCtx, credentialBindInput(claims.BindingRef, operation.TargetGeneration, payload))
		if err != nil {
			return nil, mapUsecaseError(err)
		}
		summary, err := s.managementUC.GetCredentialSummary(txCtx, bound.AccountKey)
		if err != nil {
			return nil, mapUsecaseError(err)
		}
		return successfulOperationResult(summary)
	})
	if err != nil {
		return nil, mapOperationError(err)
	}
	return &platformv2.BindCredentialResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) ReplaceCredential(ctx context.Context, req *platformv2.ReplaceCredentialRequest) (*platformv2.ReplaceCredentialResponse, error) {
	requestOperation := operationRequest(req)
	operation, claims, err := s.authorizeOperation(ctx, requestOperation, platformv2.OperationKind_OPERATION_KIND_REPLACE_CREDENTIAL, usecase.ActionCredentialUpdate, req.GetAccountKey(), operationFingerprint(requestOperation))
	if err != nil {
		return nil, err
	}
	payload, err := decodeCredentialPayload(req.GetCredentialPayloadJson())
	if err != nil {
		return nil, err
	}
	result, err := s.operationUC.Execute(ctx, operation, func(txCtx context.Context) (*biz.OperationResult, error) {
		if _, err := s.advanceGeneration(txCtx, operation, req.GetAccountKey()); err != nil {
			return nil, err
		}
		summary, err := s.managementUC.UpdateCredentialWithScope(txCtx, toScopeGuardMust(claims), usecase.UpdateCredentialInput{
			AccountKey:          req.GetAccountKey(),
			BindCredentialInput: credentialBindInput(claims.BindingRef, operation.TargetGeneration, payload),
		})
		if err != nil {
			return nil, mapUsecaseError(err)
		}
		return successfulOperationResult(summary)
	})
	if err != nil {
		return nil, mapOperationError(err)
	}
	return &platformv2.ReplaceCredentialResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) RefreshCredential(ctx context.Context, req *platformv2.RefreshCredentialRequest) (*platformv2.RefreshCredentialResponse, error) {
	requestOperation := operationRequest(req)
	operation, _, err := s.authorizeOperation(ctx, requestOperation, platformv2.OperationKind_OPERATION_KIND_REFRESH_CREDENTIAL, usecase.ActionCredentialRefresh, req.GetAccountKey(), operationFingerprint(requestOperation))
	if err != nil {
		return nil, err
	}
	result, err := s.operationUC.Execute(ctx, operation, func(txCtx context.Context) (*biz.OperationResult, error) {
		_, err := s.advanceGeneration(txCtx, operation, req.GetAccountKey())
		if err != nil {
			return nil, err
		}
		if _, err := s.statusUC.RefreshCredential(txCtx, req.GetAccountKey()); err != nil {
			return nil, mapUsecaseError(err)
		}
		if err := s.credentials.SetProfileSnapshotState(txCtx, operation.BindingRef, false, operation.TargetGeneration, operation.PreGeneration); err != nil {
			return nil, err
		}
		summary, err := s.managementUC.GetCredentialSummary(txCtx, req.GetAccountKey())
		if err != nil {
			return nil, err
		}
		return successfulOperationResult(summary)
	})
	if err != nil {
		return nil, mapOperationError(err)
	}
	return &platformv2.RefreshCredentialResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) DeleteCredential(ctx context.Context, req *platformv2.DeleteCredentialRequest) (*platformv2.DeleteCredentialResponse, error) {
	requestOperation := operationRequest(req)
	operation, claims, err := s.authorizeOperation(ctx, requestOperation, platformv2.OperationKind_OPERATION_KIND_DELETE_CREDENTIAL, usecase.ActionCredentialDelete, req.GetAccountKey(), operationFingerprint(requestOperation))
	if err != nil {
		return nil, err
	}
	result, err := s.operationUC.Execute(ctx, operation, func(txCtx context.Context) (*biz.OperationResult, error) {
		if _, err := s.advanceGeneration(txCtx, operation, req.GetAccountKey()); err != nil {
			return nil, err
		}
		if err := s.managementUC.DeleteCredentialWithScope(txCtx, toScopeGuardMust(claims), req.GetAccountKey()); err != nil {
			return nil, mapUsecaseError(err)
		}
		return &biz.OperationResult{State: "succeeded", AccountKey: req.GetAccountKey(), SnapshotJSON: `{}`}, nil
	})
	if err != nil {
		return nil, mapOperationError(err)
	}
	return &platformv2.DeleteCredentialResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) ApplyAuthorizationFence(ctx context.Context, req *platformv2.ApplyAuthorizationFenceRequest) (*platformv2.ApplyAuthorizationFenceResponse, error) {
	requestOperation := operationRequest(req)
	operation, _, err := s.authorizeOperation(
		ctx,
		requestOperation,
		platformv2.OperationKind_OPERATION_KIND_APPLY_AUTHORIZATION_FENCE,
		usecase.ActionAuthorizationFence,
		"",
		authorizationFenceFingerprint(requestOperation, req),
	)
	if err != nil {
		return nil, err
	}
	if req.GetConsumerPrincipal() == "" || req.GetMinimumGrantVersion() == 0 {
		return nil, status.Error(codes.InvalidArgument, "consumer_principal and minimum_grant_version are required")
	}
	result, err := s.operationUC.Execute(ctx, operation, func(txCtx context.Context) (*biz.OperationResult, error) {
		if s.fences == nil || s.invalidationRepo == nil {
			return nil, status.Error(codes.FailedPrecondition, "authorization fence storage is not configured")
		}
		fence := biz.AuthorizationFence{
			BindingRef:           operation.BindingRef,
			ConsumerPrincipal:    req.GetConsumerPrincipal(),
			MinimumGrantVersion:  req.GetMinimumGrantVersion(),
			MinimumOwnerEpoch:    req.GetMinimumOwnerEpoch(),
			MinimumConsumerEpoch: req.GetMinimumConsumerEpoch(),
			MinimumEntryEpoch:    req.GetMinimumEntryEpoch(),
		}
		if err := s.fences.Upsert(txCtx, fence); err != nil {
			return nil, err
		}
		if err := s.invalidationRepo.Upsert(txCtx, operation.BindingRef, req.GetConsumerPrincipal(), req.GetMinimumGrantVersion()); err != nil {
			return nil, err
		}
		credential, err := s.credentials.GetByBindingRef(txCtx, operation.BindingRef)
		if err != nil {
			return nil, err
		}
		if credential == nil || credential.Generation != operation.PreGeneration {
			return nil, status.Error(codes.Aborted, "credential generation changed concurrently")
		}
		summary, err := s.managementUC.GetCredentialSummary(txCtx, credential.AccountKey)
		if err != nil {
			return nil, err
		}
		return successfulOperationResult(summary)
	})
	if err != nil {
		return nil, mapOperationError(err)
	}
	return &platformv2.ApplyAuthorizationFenceResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) GetOperation(ctx context.Context, req *platformv2.GetOperationRequest) (*platformv2.GetOperationResponse, error) {
	if req == nil || req.GetOperationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "operation_id is required")
	}
	claims, err := authorizeTicketAction(ctx, s.ticketVerifier, usecase.ActionOperationRead, true)
	if err != nil {
		return nil, err
	}
	result, err := s.operationUC.Get(ctx, req.GetOperationId())
	if err != nil {
		return nil, mapOperationError(err)
	}
	if result == nil {
		return nil, status.Error(codes.NotFound, "operation not found")
	}
	if claims.OperationID == "" || claims.OperationID != req.GetOperationId() {
		return nil, status.Error(codes.PermissionDenied, "operation_id does not match ticket")
	}
	if result.Operation.BindingRef != claims.BindingRef {
		return nil, status.Error(codes.PermissionDenied, "operation is outside ticket scope")
	}
	return &platformv2.GetOperationResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) ResolveOperation(ctx context.Context, req *platformv2.ResolveOperationRequest) (*platformv2.ResolveOperationResponse, error) {
	requestOperation := operationRequest(req)
	operation, _, err := s.authorizeOperation(ctx, requestOperation, requestOperation.GetKind(), usecase.ActionOperationResolve, "", requestOperation.GetRequestFingerprint())
	if err != nil {
		return nil, err
	}
	result, err := s.operationUC.Resolve(ctx, operation)
	if err != nil {
		return nil, mapOperationError(err)
	}
	return &platformv2.ResolveOperationResponse{Result: toOperationResult(result)}, nil
}

func (s *PlatformControlService) GetBindingState(ctx context.Context, req *platformv2.GetBindingStateRequest) (*platformv2.GetBindingStateResponse, error) {
	if req == nil || req.GetBindingRef() == "" {
		return nil, status.Error(codes.InvalidArgument, "binding_ref is required")
	}
	claims, err := authorizeTicketAction(ctx, s.ticketVerifier, usecase.ActionBindingRead, true)
	if err != nil {
		return nil, err
	}
	if claims.BindingRef != req.GetBindingRef() {
		return nil, status.Error(codes.PermissionDenied, "binding is outside ticket scope")
	}
	credential, err := s.credentials.GetByBindingRef(ctx, req.GetBindingRef())
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	if credential == nil {
		return &platformv2.GetBindingStateResponse{State: &platformv2.BindingState{Exists: false, BindingRef: req.GetBindingRef()}}, nil
	}
	if claims.AccountKey != "" && claims.AccountKey != credential.AccountKey {
		return nil, status.Error(codes.PermissionDenied, "account_key is outside ticket scope")
	}
	summary, err := s.managementUC.GetCredentialSummary(ctx, credential.AccountKey)
	if err != nil {
		return nil, mapUsecaseError(err)
	}
	return &platformv2.GetBindingStateResponse{State: toBindingState(req.GetBindingRef(), summary)}, nil
}

func (s *PlatformControlService) authorizeOperation(ctx context.Context, request *platformv2.OperationRef, expectedKind platformv2.OperationKind, action, accountKey, expectedFingerprint string) (biz.OperationRef, *biz.ServiceTicketClaims, error) {
	if request == nil || request.GetKind() != expectedKind {
		return biz.OperationRef{}, nil, status.Error(codes.InvalidArgument, "operation kind does not match RPC")
	}
	operation, err := operationFromProto(request)
	if err != nil {
		return biz.OperationRef{}, nil, err
	}
	if expectedFingerprint == "" || request.GetRequestFingerprint() != expectedFingerprint {
		return biz.OperationRef{}, nil, status.Error(codes.InvalidArgument, "operation request fingerprint does not match payload")
	}
	claims, err := authorizeTicketAction(ctx, s.ticketVerifier, action, true)
	if err != nil {
		return biz.OperationRef{}, nil, err
	}
	if claims.BindingRef != operation.BindingRef {
		return biz.OperationRef{}, nil, status.Error(codes.PermissionDenied, "operation binding_ref does not match ticket")
	}
	if claims.OperationID == "" || claims.OperationID != operation.OperationID {
		return biz.OperationRef{}, nil, status.Error(codes.PermissionDenied, "operation_id does not match ticket")
	}
	if claims.CredentialGeneration != operation.PreGeneration {
		return biz.OperationRef{}, nil, status.Error(codes.PermissionDenied, "credential generation does not match ticket")
	}
	if accountKey != "" && claims.AccountKey != accountKey {
		return biz.OperationRef{}, nil, status.Error(codes.PermissionDenied, "account_key does not match ticket")
	}
	return operation, claims, nil
}

func (s *PlatformControlService) advanceGeneration(ctx context.Context, operation biz.OperationRef, accountKey string) (*biz.Credential, error) {
	credential, err := s.credentials.GetByBindingRef(ctx, operation.BindingRef)
	if err != nil {
		return nil, err
	}
	if credential == nil {
		return nil, status.Error(codes.NotFound, "credential not found")
	}
	if accountKey == "" {
		accountKey = credential.AccountKey
	}
	if credential.AccountKey != accountKey {
		return nil, status.Error(codes.PermissionDenied, "account_key is outside operation scope")
	}
	updated, err := s.credentials.AdvanceGeneration(ctx, operation.BindingRef, accountKey, operation.PreGeneration, operation.TargetGeneration)
	if errors.Is(err, biz.ErrCredentialGenerationConflict) {
		return nil, status.Error(codes.Aborted, "credential generation changed concurrently")
	}
	return updated, err
}

func mapOperationError(err error) error {
	if status.Code(err) != codes.Unknown {
		return err
	}
	if errors.Is(err, biz.ErrOperationConflict) {
		return status.Error(codes.AlreadyExists, "operation_id is already bound to different input")
	}
	if errors.Is(err, biz.ErrOperationState) || errors.Is(err, usecase.ErrOperationRequired) {
		return status.Error(codes.FailedPrecondition, "operation state is invalid")
	}
	return mapUsecaseError(err)
}

func operationRequest(request interface {
	GetOperation() *platformv2.OperationRef
}) *platformv2.OperationRef {
	if request == nil {
		return nil
	}
	return request.GetOperation()
}

func operationFingerprint(operation *platformv2.OperationRef) string {
	if operation == nil {
		return ""
	}
	return operationid.Fingerprint(
		operation.GetKind().String(),
		operation.GetBindingRef(),
		operation.GetPreGeneration(),
		operation.GetTargetGeneration(),
	)
}

func authorizationFenceFingerprint(operation *platformv2.OperationRef, request *platformv2.ApplyAuthorizationFenceRequest) string {
	if operation == nil || request == nil {
		return ""
	}
	return operationid.AuthorizationFenceFingerprint(
		operation.GetKind().String(), operation.GetBindingRef(), request.GetConsumerPrincipal(), operation.GetPreGeneration(),
		request.GetMinimumGrantVersion(), request.GetMinimumOwnerEpoch(), request.GetMinimumConsumerEpoch(), request.GetMinimumEntryEpoch(),
	)
}

type credentialPayload struct {
	CookieBundle string `json:"cookie_bundle"`
	DeviceID     string `json:"device_id"`
	DeviceFP     string `json:"device_fp"`
	DeviceName   string `json:"device_name"`
	RegionHint   string `json:"region_hint"`
}

func decodeCredentialPayload(raw string) (*credentialPayload, error) {
	var payload credentialPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, status.Error(codes.InvalidArgument, "credential payload must be valid JSON")
	}
	if payload.CookieBundle == "" || payload.DeviceID == "" || payload.DeviceFP == "" {
		return nil, status.Error(codes.InvalidArgument, "credential payload is incomplete")
	}
	return &payload, nil
}

func credentialBindInput(bindingRef string, generation uint64, payload *credentialPayload) usecase.BindCredentialInput {
	return usecase.BindCredentialInput{
		BindingRef:       bindingRef,
		Generation:       generation,
		CookieBundleJSON: payload.CookieBundle,
		DeviceID:         payload.DeviceID,
		DeviceFP:         payload.DeviceFP,
		DeviceName:       payload.DeviceName,
		RegionHint:       payload.RegionHint,
	}
}

func operationFromProto(operation *platformv2.OperationRef) (biz.OperationRef, error) {
	if operation == nil || operation.GetOperationId() == "" || operation.GetBindingRef() == "" || operation.GetRequestFingerprint() == "" || operation.GetKind() == platformv2.OperationKind_OPERATION_KIND_UNSPECIFIED {
		return biz.OperationRef{}, status.Error(codes.InvalidArgument, "complete operation reference is required")
	}
	return biz.OperationRef{
		OperationID:        operation.GetOperationId(),
		Kind:               operation.GetKind().String(),
		BindingRef:         operation.GetBindingRef(),
		PreGeneration:      operation.GetPreGeneration(),
		TargetGeneration:   operation.GetTargetGeneration(),
		RequestFingerprint: operation.GetRequestFingerprint(),
	}, nil
}

func successfulOperationResult(summary *usecase.CredentialSummaryOutput) (*biz.OperationResult, error) {
	snapshot, err := json.Marshal(storedSnapshotFromSummary(summary))
	if err != nil {
		return nil, err
	}
	return &biz.OperationResult{
		State:        "succeeded",
		AccountKey:   summary.AccountKey,
		Status:       string(summary.Status),
		SnapshotJSON: string(snapshot),
	}, nil
}
