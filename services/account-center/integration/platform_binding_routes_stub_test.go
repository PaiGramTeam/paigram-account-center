//go:build integration

package integration

import (
	"context"
	"net"
	"testing"

	platformv1 "github.com/PaiGramTeam/paigram-account-center/contracts/gen/go/platform/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type platformBindingRouteStub struct {
	platformv1.UnimplementedPlatformServiceServer
	summaryResponse           *platformv1.GetCredentialSummaryResponse
	credentialMutationSummary *platformv1.GetCredentialSummaryResponse
	credentialMutationErr     error
	deleteErr                 error
	confirmPrimaryProfileErr  error
	lastBind                  *platformv1.BindCredentialRequest
	lastReplace               *platformv1.ReplaceCredentialRequest
	deleteRequests            []*platformv1.DeleteCredentialRequest
	lastConfirmPrimaryProfile *platformv1.ConfirmPrimaryProfileRequest
	lastConfirmAuthorization  []string
}

func (s *platformBindingRouteStub) DescribePlatform(context.Context, *platformv1.DescribePlatformRequest) (*platformv1.DescribePlatformResponse, error) {
	return &platformv1.DescribePlatformResponse{}, nil
}

func (s *platformBindingRouteStub) GetCredentialSummary(context.Context, *platformv1.GetCredentialSummaryRequest) (*platformv1.GetCredentialSummaryResponse, error) {
	if s.summaryResponse != nil {
		return s.summaryResponse, nil
	}
	return &platformv1.GetCredentialSummaryResponse{}, nil
}

func (s *platformBindingRouteStub) BindCredential(_ context.Context, req *platformv1.BindCredentialRequest) (*platformv1.BindCredentialResponse, error) {
	s.lastBind = req
	if s.credentialMutationErr != nil {
		return nil, s.credentialMutationErr
	}
	return &platformv1.BindCredentialResponse{Summary: s.credentialMutationSummary}, nil
}

func (s *platformBindingRouteStub) ReplaceCredential(_ context.Context, req *platformv1.ReplaceCredentialRequest) (*platformv1.ReplaceCredentialResponse, error) {
	s.lastReplace = req
	if s.credentialMutationErr != nil {
		return nil, s.credentialMutationErr
	}
	return &platformv1.ReplaceCredentialResponse{Summary: s.credentialMutationSummary}, nil
}

func (s *platformBindingRouteStub) RefreshCredential(context.Context, *platformv1.RefreshCredentialRequest) (*platformv1.RefreshCredentialResponse, error) {
	return &platformv1.RefreshCredentialResponse{}, nil
}

func (s *platformBindingRouteStub) DeleteCredential(_ context.Context, req *platformv1.DeleteCredentialRequest) (*platformv1.DeleteCredentialResponse, error) {
	s.deleteRequests = append(s.deleteRequests, req)
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	return &platformv1.DeleteCredentialResponse{Success: true}, nil
}

func (s *platformBindingRouteStub) InvalidateConsumerGrant(context.Context, *platformv1.InvalidateConsumerGrantRequest) (*platformv1.InvalidateConsumerGrantResponse, error) {
	return &platformv1.InvalidateConsumerGrantResponse{Success: true}, nil
}

func (s *platformBindingRouteStub) ConfirmPrimaryProfile(ctx context.Context, req *platformv1.ConfirmPrimaryProfileRequest) (*platformv1.ConfirmPrimaryProfileResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.lastConfirmAuthorization = append([]string(nil), md.Get("authorization")...)
	s.lastConfirmPrimaryProfile = req
	if s.confirmPrimaryProfileErr != nil {
		return nil, s.confirmPrimaryProfileErr
	}
	return &platformv1.ConfirmPrimaryProfileResponse{}, nil
}

func startPlatformBindingRouteServer(t *testing.T, stub *platformBindingRouteStub) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	platformv1.RegisterPlatformServiceServer(server, stub)
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
