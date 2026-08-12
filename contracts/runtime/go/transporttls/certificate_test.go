package transporttls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testAuthority struct {
	certificate    *x509.Certificate
	privateKey     ed25519.PrivateKey
	certificatePEM []byte
}

type testCertificate struct {
	certificatePEM []byte
	privateKeyPEM  []byte
}

func newTestAuthority(t *testing.T, commonName string, now time.Time) testAuthority {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testAuthority{
		certificate:    certificate,
		privateKey:     privateKey,
		certificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

func (a testAuthority) issue(t *testing.T, commonName string, dnsNames []string, usages []x509.ExtKeyUsage, notBefore, notAfter time.Time) testCertificate {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: randomSerial(t),
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     dnsNames,
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usages,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.certificate, publicKey, a.privateKey)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return testCertificate{
		certificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		privateKeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
	}
}

func writeTestTLSFiles(t *testing.T, authority testAuthority, certificate testCertificate, prefix string) (string, string, string) {
	t.Helper()
	directory := t.TempDir()
	caFile := filepath.Join(directory, prefix+"-ca.pem")
	certificateFile := filepath.Join(directory, prefix+"-cert.pem")
	privateKeyFile := filepath.Join(directory, prefix+"-key.pem")
	require.NoError(t, os.WriteFile(caFile, authority.certificatePEM, 0o600))
	require.NoError(t, os.WriteFile(certificateFile, certificate.certificatePEM, 0o600))
	require.NoError(t, os.WriteFile(privateKeyFile, certificate.privateKeyPEM, 0o600))
	return caFile, certificateFile, privateKeyFile
}

func appendCertificate(t *testing.T, path string, certificatePEM []byte) {
	t.Helper()
	existing, err := os.ReadFile(path)
	require.NoError(t, err)
	existing = append(existing, certificatePEM...)
	require.NoError(t, os.WriteFile(path, existing, 0o600))
}

func overwriteFile(t *testing.T, path string, value []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, value, 0o600))
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	require.NoError(t, err)
	return serial
}

func tlsHandshake(serverConfig, clientConfig *tls.Config) error {
	serverConnection, clientConnection := net.Pipe()
	deadline := time.Now().Add(5 * time.Second)
	_ = serverConnection.SetDeadline(deadline)
	_ = clientConnection.SetDeadline(deadline)
	serverTLS := tls.Server(serverConnection, serverConfig)
	clientTLS := tls.Client(clientConnection, clientConfig)
	defer serverConnection.Close()
	defer clientConnection.Close()
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- serverTLS.Handshake()
	}()
	clientErr := clientTLS.Handshake()
	_ = clientConnection.Close()
	serverErr := <-serverResult
	if clientErr != nil {
		return clientErr
	}
	return serverErr
}
