package tlstest

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateRSACreatesMutuallyTrustedCertificates(t *testing.T) {
	serverName := "platform-control.internal"
	bundle, err := GenerateRSA(filepath.Join(t.TempDir(), "tls"), serverName)
	if err != nil {
		t.Fatalf("GenerateRSA() error = %v", err)
	}
	if bundle.ServerName != serverName {
		t.Fatalf("ServerName = %q, want %q", bundle.ServerName, serverName)
	}
	serverPair, err := tls.LoadX509KeyPair(bundle.ServerCertFile, bundle.ServerKeyFile)
	if err != nil {
		t.Fatalf("load server key pair: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(bundle.ClientCertFile, bundle.ClientKeyFile); err != nil {
		t.Fatalf("load client key pair: %v", err)
	}
	serverCertificate, err := x509.ParseCertificate(serverPair.Certificate[0])
	if err != nil {
		t.Fatalf("parse server certificate: %v", err)
	}
	caPEM, err := os.ReadFile(bundle.CAFile)
	if err != nil {
		t.Fatalf("read CA certificate: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("append CA certificate")
	}
	if _, err := serverCertificate.Verify(x509.VerifyOptions{
		DNSName: serverName,
		Roots:   roots,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		CurrentTime: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("verify server certificate: %v", err)
	}
}

func TestGenerateRSARejectsMissingInputs(t *testing.T) {
	for name, values := range map[string][2]string{
		"directory":   {"", "platform-control.internal"},
		"server name": {t.TempDir(), ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := GenerateRSA(values[0], values[1]); err == nil {
				t.Fatal("GenerateRSA() error = nil")
			}
		})
	}
}
