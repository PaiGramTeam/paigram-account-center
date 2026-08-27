package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/transporttls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type healthcheckOptions struct {
	Target     string
	RootCAFile string
	ServerName string
	Service    string
	Timeout    time.Duration
}

func main() {
	options := healthcheckOptions{}
	flag.StringVar(&options.Target, "target", "127.0.0.1:9001", "runtime gRPC target")
	flag.StringVar(&options.RootCAFile, "root-ca", "", "runtime TLS root CA file")
	flag.StringVar(&options.ServerName, "server-name", "", "runtime TLS server name")
	flag.StringVar(&options.Service, "service", "", "gRPC health service name; empty checks readiness")
	flag.DurationVar(&options.Timeout, "timeout", 3*time.Second, "health check timeout")
	flag.Parse()

	if err := checkRuntimeHealth(context.Background(), options); err != nil {
		log.Printf("runtime readiness failed: %v", err)
		os.Exit(1)
	}
}

func checkRuntimeHealth(ctx context.Context, options healthcheckOptions) error {
	if options.Target == "" {
		return errors.New("target is required")
	}
	if options.Timeout <= 0 {
		return errors.New("timeout must be greater than zero")
	}
	loader, err := transporttls.NewOptionalClientConfigLoader(transporttls.ClientFiles{
		RootCAFile: options.RootCAFile,
		ServerName: options.ServerName,
	})
	if err != nil {
		return fmt.Errorf("load runtime TLS configuration: %w", err)
	}
	transportCredentials := credentials.TransportCredentials(insecure.NewCredentials())
	if loader != nil {
		tlsConfig, loadErr := loader.Load()
		if loadErr != nil {
			return fmt.Errorf("load runtime TLS identity: %w", loadErr)
		}
		transportCredentials = credentials.NewTLS(tlsConfig)
	}
	checkCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	connection, err := grpc.DialContext(
		checkCtx,
		options.Target,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithBlock(),
		grpc.WithReturnConnectionError(),
	)
	if err != nil {
		return fmt.Errorf("dial runtime gRPC: %w", err)
	}
	defer connection.Close()

	response, err := healthpb.NewHealthClient(connection).Check(checkCtx, &healthpb.HealthCheckRequest{Service: options.Service})
	if err != nil {
		return fmt.Errorf("check runtime gRPC health: %w", err)
	}
	if response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("runtime gRPC status is %s", response.GetStatus())
	}
	return nil
}
