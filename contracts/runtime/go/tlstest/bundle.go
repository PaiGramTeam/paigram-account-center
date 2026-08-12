package tlstest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type Bundle struct {
	CAFile            string
	ServerName        string
	ServerCertFile    string
	ServerKeyFile     string
	ClientCertFile    string
	ClientKeyFile     string
	CACertificatePEM  []byte
	ServerCertificate []byte
	ServerPrivateKey  []byte
}

func New(t testing.TB, serverName string) Bundle {
	t.Helper()
	now := time.Now().UTC()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serial(t),
		Subject:               pkix.Name{CommonName: serverName + " test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	serverCertificate, serverPrivateKey := issue(t, caCertificate, caPrivate, serverName, []string{serverName}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now)
	clientCertificate, clientPrivateKey := issue(t, caCertificate, caPrivate, "account-center", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, now)
	directory := t.TempDir()
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
	write(t, bundle.CAFile, bundle.CACertificatePEM)
	write(t, bundle.ServerCertFile, serverCertificate)
	write(t, bundle.ServerKeyFile, serverPrivateKey)
	write(t, bundle.ClientCertFile, clientCertificate)
	write(t, bundle.ClientKeyFile, clientPrivateKey)
	return bundle
}

func issue(t testing.TB, authority *x509.Certificate, authorityKey ed25519.PrivateKey, commonName string, dnsNames []string, usages []x509.ExtKeyUsage, now time.Time) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial(t),
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     dnsNames,
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usages,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, authority, publicKey, authorityKey)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal leaf private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
}

func serial(t testing.TB) *big.Int {
	t.Helper()
	value, err := randomSerial()
	if err != nil {
		t.Fatalf("generate certificate serial: %v", err)
	}
	return value
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
}

func write(t testing.TB, path string, value []byte) {
	t.Helper()
	if err := os.WriteFile(path, value, 0o600); err != nil {
		t.Fatalf("write TLS fixture: %v", err)
	}
}
