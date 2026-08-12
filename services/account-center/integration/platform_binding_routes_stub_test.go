//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"testing"

	platformv2 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v2"
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type platformBindingRouteStub struct {
	platformv2.UnimplementedPlatformControlServiceServer
	summaryResponse           *routeCredentialSummary
	credentialMutationSummary *routeCredentialSummary
	credentialMutationErr     error
	deleteErr                 error
	authorizationFenceErr     error
	lastBind                  *platformv2.BindCredentialRequest
	lastReplace               *platformv2.ReplaceCredentialRequest
	lastPrimary               *platformv2.SetPrimaryProfileRequest
	deleteRequests            []*platformv2.DeleteCredentialRequest
}

type routeCredentialSummary struct {
	PlatformAccountId string
	Status            platformv2.CredentialStatus
	LastValidatedAt   *timestamppb.Timestamp
	LastRefreshedAt   *timestamppb.Timestamp
	Profiles          []*routeProfileSummary
}

type routeProfileSummary struct {
	Id                uint64
	PlatformAccountId string
	GameBiz           string
	Region            string
	PlayerId          string
	Nickname          string
	Level             int32
	IsDefault         bool
}

func (s *platformBindingRouteStub) GetBindingState(_ context.Context, req *platformv2.GetBindingStateRequest) (*platformv2.GetBindingStateResponse, error) {
	if s.summaryResponse != nil {
		state := routeSummaryToBindingState(s.summaryResponse)
		state.BindingRef = req.GetBindingRef()
		return &platformv2.GetBindingStateResponse{State: state}, nil
	}
	return &platformv2.GetBindingStateResponse{}, nil
}

func (s *platformBindingRouteStub) BindCredential(_ context.Context, req *platformv2.BindCredentialRequest) (*platformv2.BindCredentialResponse, error) {
	s.lastBind = req
	if s.credentialMutationErr != nil {
		return nil, s.credentialMutationErr
	}
	return &platformv2.BindCredentialResponse{Result: operationResultForRequest(routeSummaryToOperationResult(s.credentialMutationSummary), req.GetOperation())}, nil
}

func (s *platformBindingRouteStub) ReplaceCredential(_ context.Context, req *platformv2.ReplaceCredentialRequest) (*platformv2.ReplaceCredentialResponse, error) {
	s.lastReplace = req
	if s.credentialMutationErr != nil {
		return nil, s.credentialMutationErr
	}
	return &platformv2.ReplaceCredentialResponse{Result: operationResultForRequest(routeSummaryToOperationResult(s.credentialMutationSummary), req.GetOperation())}, nil
}

func (s *platformBindingRouteStub) RefreshCredential(_ context.Context, req *platformv2.RefreshCredentialRequest) (*platformv2.RefreshCredentialResponse, error) {
	return &platformv2.RefreshCredentialResponse{Result: operationResultForRequest(routeSummaryToOperationResult(s.credentialMutationSummary), req.GetOperation())}, nil
}

func (s *platformBindingRouteStub) SetPrimaryProfile(_ context.Context, req *platformv2.SetPrimaryProfileRequest) (*platformv2.SetPrimaryProfileResponse, error) {
	s.lastPrimary = req
	if s.credentialMutationErr != nil {
		return nil, s.credentialMutationErr
	}
	state := routeSummaryToBindingState(s.summaryResponse)
	if state == nil {
		return &platformv2.SetPrimaryProfileResponse{}, nil
	}
	for _, profile := range state.GetProfileSnapshot().GetProfiles() {
		profile.IsDefault = profile.GetProfileRef() == req.GetProfileRef()
	}
	state.ProfileSnapshot.Revision = req.GetExpectedProfileRevision() + 1
	state.ProfileSnapshot.ObservedRevision = req.GetExpectedProfileRevision() + 1
	result := &platformv2.OperationResult{
		AccountKey: state.GetAccountKey(), CredentialStatus: state.GetCredentialStatus(), ProfileSnapshot: state.GetProfileSnapshot(),
		LastValidatedAt: state.GetLastValidatedAt(), LastRefreshedAt: state.GetLastRefreshedAt(),
	}
	return &platformv2.SetPrimaryProfileResponse{Result: operationResultForRequest(result, req.GetOperation())}, nil
}

func routeSummaryToOperationResult(summary *routeCredentialSummary) *platformv2.OperationResult {
	if summary == nil {
		return nil
	}
	return &platformv2.OperationResult{
		AccountKey:       summary.PlatformAccountId,
		CredentialStatus: summary.Status,
		ProfileSnapshot:  routeProfileSnapshot(summary.Profiles),
		LastValidatedAt:  summary.LastValidatedAt,
		LastRefreshedAt:  summary.LastRefreshedAt,
	}
}

func routeSummaryToBindingState(summary *routeCredentialSummary) *platformv2.BindingState {
	if summary == nil {
		return nil
	}
	return &platformv2.BindingState{
		Exists:           true,
		AccountKey:       summary.PlatformAccountId,
		CredentialStatus: summary.Status,
		ProfileSnapshot:  routeProfileSnapshot(summary.Profiles),
		LastValidatedAt:  summary.LastValidatedAt,
		LastRefreshedAt:  summary.LastRefreshedAt,
	}
}

func routeProfileSnapshot(profiles []*routeProfileSummary) *platformv2.ProfileSnapshot {
	items := make([]*platformv2.ProfileSummary, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, &platformv2.ProfileSummary{
			ProfileRef: fmt.Sprintf("mihomo:%d", profile.Id),
			AccountKey: profile.PlatformAccountId,
			GameBiz:    profile.GameBiz,
			Region:     profile.Region,
			PlayerId:   profile.PlayerId,
			Nickname:   profile.Nickname,
			Level:      profile.Level,
			IsDefault:  profile.IsDefault,
		})
	}
	return &platformv2.ProfileSnapshot{Profiles: items, Complete: true, Revision: 1, ObservedRevision: 1}
}

func (s *platformBindingRouteStub) DeleteCredential(_ context.Context, req *platformv2.DeleteCredentialRequest) (*platformv2.DeleteCredentialResponse, error) {
	s.deleteRequests = append(s.deleteRequests, req)
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	return &platformv2.DeleteCredentialResponse{Result: operationResultForRequest(&platformv2.OperationResult{
		State: platformv2.OperationState_OPERATION_STATE_SUCCEEDED,
	}, req.GetOperation())}, nil
}

func (s *platformBindingRouteStub) ApplyAuthorizationFence(_ context.Context, req *platformv2.ApplyAuthorizationFenceRequest) (*platformv2.ApplyAuthorizationFenceResponse, error) {
	if s.authorizationFenceErr != nil {
		return nil, s.authorizationFenceErr
	}
	return &platformv2.ApplyAuthorizationFenceResponse{Result: operationResultForRequest(&platformv2.OperationResult{
		State: platformv2.OperationState_OPERATION_STATE_SUCCEEDED,
	}, req.GetOperation())}, nil
}

func operationResultForRequest(result *platformv2.OperationResult, operation *platformv2.OperationRef) *platformv2.OperationResult {
	if result == nil {
		return nil
	}
	copy := proto.Clone(result).(*platformv2.OperationResult)
	if copy.Operation == nil {
		copy.Operation = operation
	}
	if copy.State == platformv2.OperationState_OPERATION_STATE_UNSPECIFIED {
		copy.State = platformv2.OperationState_OPERATION_STATE_SUCCEEDED
	}
	return copy
}

func startPlatformBindingRouteServer(t *testing.T, stack *integrationStack, stub *platformBindingRouteStub) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsConfig, err := transporttls.NewServerConfig(transporttls.ServerFiles{
		CertificateFile: stack.ControlTLS.ServerCertFile,
		PrivateKeyFile:  stack.ControlTLS.ServerKeyFile,
		ClientCAFile:    stack.ControlTLS.CAFile,
	}, transporttls.MutualTLS)
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	platformv2.RegisterPlatformControlServiceServer(server, stub)
	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		<-serveErrCh
	})
	return listener.Addr().String()
}
