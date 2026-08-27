package platformtransport

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

var ErrInvalidControlTransport = errors.New("invalid platform control transport configuration")
var ErrControlTransportNotConfigured = errors.New("platform control transport is not configured")

type ControlConfig struct {
	RootCAFile      string
	CertificateFile string
	PrivateKeyFile  string
	ServerName      string
	Timeout         time.Duration
}

type DialFunc func(context.Context, string) (*grpc.ClientConn, error)

func NewControlDialer(config ControlConfig) (DialFunc, error) {
	if config.Timeout <= 0 {
		return nil, ErrInvalidControlTransport
	}
	files := transporttls.ClientFiles{
		RootCAFile:      config.RootCAFile,
		CertificateFile: config.CertificateFile,
		PrivateKeyFile:  config.PrivateKeyFile,
		ServerName:      strings.TrimSpace(config.ServerName),
	}
	loader, err := transporttls.NewOptionalClientConfigLoader(files)
	if err != nil {
		return nil, errors.Join(ErrInvalidControlTransport, err)
	}
	return func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		if err := validateEndpoint(endpoint); err != nil {
			return nil, err
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dialContext, cancel := context.WithTimeout(ctx, config.Timeout)
		defer cancel()
		transportCredentials := credentials.TransportCredentials(insecure.NewCredentials())
		dialOptions := []grpc.DialOption{grpc.WithBlock()}
		if loader != nil {
			tlsConfig, err := loader.Load()
			if err != nil {
				return nil, errors.Join(ErrInvalidControlTransport, err)
			}
			transportCredentials = credentials.NewTLS(tlsConfig)
			dialOptions = append(dialOptions, grpc.WithAuthority(files.ServerName))
		}
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(transportCredentials))
		return grpc.DialContext(
			dialContext,
			endpoint,
			dialOptions...,
		)
	}, nil
}

func validateEndpoint(endpoint string) error {
	if endpoint == "" || strings.TrimSpace(endpoint) != endpoint || strings.Contains(endpoint, "://") || strings.ContainsAny(endpoint, "/?#") {
		return ErrInvalidControlTransport
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || strings.TrimSpace(host) == "" {
		return ErrInvalidControlTransport
	}
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsedPort == 0 {
		return ErrInvalidControlTransport
	}
	return nil
}
