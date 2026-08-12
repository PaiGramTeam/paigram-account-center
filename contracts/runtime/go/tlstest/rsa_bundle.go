package tlstest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// NewRSA creates a TLS fixture that can be consumed by runtimes whose TLS
// stack does not negotiate Ed25519 certificate signatures.
func NewRSA(t testing.TB, serverName string) Bundle {
	t.Helper()
	bundle, err := GenerateRSA(t.TempDir(), serverName)
	if err != nil {
		t.Fatalf("generate RSA TLS fixture: %v", err)
	}
	return bundle
}

func GenerateRSA(directory, serverName string) (Bundle, error) {
	if directory == "" || serverName == "" {
		return Bundle{}, fmt.Errorf("TLS fixture directory and server name are required")
	}
	now := time.Now().UTC()
	caPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return Bundle{}, fmt.Errorf("generate RSA CA key: %w", err)
	}
	serialNumber, err := randomSerial()
	if err != nil {
		return Bundle{}, fmt.Errorf("generate RSA CA serial: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: serverName + " RSA test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivate.PublicKey, caPrivate)
	if err != nil {
		return Bundle{}, fmt.Errorf("create RSA CA certificate: %w", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		return Bundle{}, fmt.Errorf("parse RSA CA certificate: %w", err)
	}
	serverCertificate, serverPrivateKey, err := issueRSA(caCertificate, caPrivate, serverName, []string{serverName}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now)
	if err != nil {
		return Bundle{}, err
	}
	clientCertificate, clientPrivateKey, err := issueRSA(caCertificate, caPrivate, "account-center", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, now)
	if err != nil {
		return Bundle{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Bundle{}, fmt.Errorf("create TLS fixture directory: %w", err)
	}
	bundle := Bundle{
		CAFile:            filepath.Join(directory, "ca.pem"),
		ServerName:        serverName,
		ServerCertFile:    filepath.Join(directory, "server-cert.pem"),
		ServerKeyFile:     filepath.Join(directory, "server-key.pem"),
		ClientCertFile:    filepath.Join(directory, "client-cert.pem"),
		ClientKeyFile:     filepath.Join(directory, "client-key.pem"),
		CACertificatePEM:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		ServerCertificate: serverCertificate,
		ServerPrivateKey:  serverPrivateKey,
	}
	for path, value := range map[string][]byte{
		bundle.CAFile:         bundle.CACertificatePEM,
		bundle.ServerCertFile: serverCertificate,
		bundle.ServerKeyFile:  serverPrivateKey,
		bundle.ClientCertFile: clientCertificate,
		bundle.ClientKeyFile:  clientPrivateKey,
	} {
		if err := os.WriteFile(path, value, 0o600); err != nil {
			return Bundle{}, fmt.Errorf("write TLS fixture %s: %w", filepath.Base(path), err)
		}
	}
	return bundle, nil
}

func issueRSA(authority *x509.Certificate, authorityKey *rsa.PrivateKey, commonName string, dnsNames []string, usages []x509.ExtKeyUsage, now time.Time) ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA leaf key: %w", err)
	}
	serialNumber, err := randomSerial()
	if err != nil {
		return nil, nil, fmt.Errorf("generate RSA leaf serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     dnsNames,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usages,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, authority, &privateKey.PublicKey, authorityKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create RSA leaf certificate: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}), nil
}
