package usecase

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"platform-mihomo-service/internal/biz"
)

var ErrOperationRequired = errors.New("operation reference is required")

type OperationUsecase struct {
	repository biz.OperationRepository
}

func NewOperationUsecase(repository biz.OperationRepository) *OperationUsecase {
	return &OperationUsecase{repository: repository}
}

func (uc *OperationUsecase) Execute(
	ctx context.Context,
	operation biz.OperationRef,
	mutation func(context.Context) (*biz.OperationResult, error),
) (*biz.OperationResult, error) {
	if err := validateOperationRef(operation); err != nil {
		return nil, err
	}
	existing, admitted, err := uc.repository.Admit(ctx, operation)
	if err != nil {
		return nil, err
	}
	if !admitted {
		return existing, nil
	}
	if existing == nil || existing.ExecutionToken == "" {
		return nil, biz.ErrOperationState
	}
	var output *biz.OperationResult
	err = uc.repository.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := uc.repository.LockPending(txCtx, operation.OperationID, existing.ExecutionToken); err != nil {
			return err
		}
		result, err := mutation(txCtx)
		if err != nil {
			return err
		}
		if result == nil {
			return biz.ErrOperationState
		}
		result.Operation = operation
		result.ExecutionToken = existing.ExecutionToken
		if err := uc.repository.Complete(txCtx, *result); err != nil {
			return err
		}
		output, err = uc.repository.Get(txCtx, operation.OperationID)
		return err
	})
	if err != nil {
		_ = uc.repository.FailPending(ctx, operation.OperationID, existing.ExecutionToken, operationFailureReason(err))
	}
	return output, err
}

func operationFailureReason(err error) string {
	switch status.Code(err) {
	case codes.InvalidArgument:
		return "invalid_input"
	case codes.PermissionDenied, codes.Unauthenticated:
		return "permission_denied"
	case codes.NotFound:
		return "not_found"
	case codes.AlreadyExists:
		return "conflict"
	case codes.FailedPrecondition:
		return "failed_precondition"
	case codes.Aborted:
		return "generation_conflict"
	case codes.DeadlineExceeded:
		return "deadline_exceeded"
	case codes.Unavailable:
		return "dependency_unavailable"
	default:
		return "internal_failure"
	}
}

func (uc *OperationUsecase) Get(ctx context.Context, operationID string) (*biz.OperationResult, error) {
	if operationID == "" {
		return nil, ErrOperationRequired
	}
	return uc.repository.Get(ctx, operationID)
}

func (uc *OperationUsecase) Resolve(ctx context.Context, operation biz.OperationRef) (*biz.OperationResult, error) {
	if err := validateOperationRef(operation); err != nil {
		return nil, err
	}
	return uc.repository.Resolve(ctx, operation)
}

func validateOperationRef(operation biz.OperationRef) error {
	if operation.OperationID == "" || operation.Kind == "" || operation.BindingRef == "" || operation.RequestFingerprint == "" {
		return ErrOperationRequired
	}
	if operation.Kind == "OPERATION_KIND_APPLY_AUTHORIZATION_FENCE" {
		if operation.TargetGeneration != operation.PreGeneration {
			return ErrOperationRequired
		}
	} else if operation.TargetGeneration != operation.PreGeneration+1 {
		return ErrOperationRequired
	}
	return nil
}
