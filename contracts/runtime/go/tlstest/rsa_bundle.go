package tlstest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"path/filepath"
	"testing"
	"time"
)

// NewRSA creates a TLS fixture that can be consumed by runtimes whose TLS
// stack does not negotiate Ed25519 certificate signatures.
func NewRSA(t testing.TB, serverName string) Bundle {
	t.Helper()
	now := time.Now().UTC()
	caPrivate, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serial(t),
		Subject:               pkix.Name{CommonName: serverName + " RSA test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivate.PublicKey, caPrivate)
	if err != nil {
		t.Fatalf("create RSA CA certificate: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse RSA CA certificate: %v", err)
	}
	serverCertificate, serverPrivateKey := issueRSA(t, caCertificate, caPrivate, serverName, []string{serverName}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now)
	clientCertificate, clientPrivateKey := issueRSA(t, caCertificate, caPrivate, "account-center", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, now)
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

func issueRSA(t testing.TB, authority *x509.Certificate, authorityKey *rsa.PrivateKey, commonName string, dnsNames []string, usages []x509.ExtKeyUsage, now time.Time) ([]byte, []byte) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA leaf key: %v", err)
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
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, authority, &privateKey.PublicKey, authorityKey)
	if err != nil {
		t.Fatalf("create RSA leaf certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
}
