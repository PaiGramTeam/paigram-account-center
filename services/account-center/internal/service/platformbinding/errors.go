package platformbinding

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"paigram/internal/model"
)

var (
	ErrBindingAlreadyOwned             = errors.New("platform binding is already owned by another user")
	ErrBindingRuntimeSummaryNotReady   = errors.New("platform binding runtime summary is not ready")
	ErrBindingNotFound                 = errors.New("platform binding not found")
	ErrCredentialGatewayUnavailable    = errors.New("platform credential orchestration is unavailable")
	ErrCredentialOperationPending      = errors.New("platform credential operation is pending reconciliation")
	ErrCredentialValidationFailed      = errors.New("platform credential validation failed")
	ErrConsumerNotSupported            = errors.New("consumer is not supported")
	ErrGrantActionNotAllowed           = errors.New("consumer grant action is not allowed")
	ErrGrantPropagationPending         = errors.New("consumer grant authorization fence propagation is pending")
	ErrBindingGenerationConflict       = errors.New("platform binding generation changed concurrently")
	ErrMultiplePrimaryProfiles         = errors.New("multiple primary profiles are not supported")
	ErrPlatformServiceUnavailable      = errors.New("platform service is unavailable")
	ErrPlatformSummaryProxyUnavailable = errors.New("platform summary proxy is unavailable")
	ErrPrimaryProfileNotOwned          = errors.New("primary profile must belong to binding")
)

type CredentialOperationPendingError struct {
	OperationID string
	BindingID   uint64
	State       model.PlatformOperationIntentState
}

type GrantPropagationPendingError struct {
	BindingID           uint64
	Consumer            string
	MinimumGrantVersion uint64
	Cause               error
}

func (e *GrantPropagationPendingError) Error() string {
	return ErrGrantPropagationPending.Error()
}

func (e *GrantPropagationPendingError) Unwrap() error {
	return e.Cause
}

func (e *GrantPropagationPendingError) Is(target error) bool {
	return target == ErrGrantPropagationPending || errors.Is(e.Cause, target)
}

func (e *CredentialOperationPendingError) Error() string {
	return ErrCredentialOperationPending.Error()
}

func (e *CredentialOperationPendingError) Unwrap() error {
	return ErrCredentialOperationPending
}

func IsExecutionPlaneUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCredentialGatewayUnavailable) || errors.Is(err, ErrPlatformServiceUnavailable) || errors.Is(err, ErrPlatformSummaryProxyUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if st, ok := grpcstatus.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") || strings.Contains(msg, "dial")
}

func IsCredentialValidationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCredentialValidationFailed) {
		return true
	}
	if st, ok := grpcstatus.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument, codes.FailedPrecondition, codes.PermissionDenied, codes.Unauthenticated:
			return true
		}
	}
	return false
}
