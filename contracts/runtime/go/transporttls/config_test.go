package transporttls

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestControlTLSRequiresTrustedClientCertificate(t *testing.T) {
	now := time.Now().UTC()
	authority := newTestAuthority(t, "control-ca", now)
	otherAuthority := newTestAuthority(t, "other-ca", now)
	serverCertificate := authority.issue(t, "control", []string{"control.internal"}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now.Add(-time.Minute), now.Add(time.Hour))
	clientCertificate := authority.issue(t, "account-center", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, now.Add(-time.Minute), now.Add(time.Hour))
	untrustedClientCertificate := otherAuthority.issue(t, "intruder", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, now.Add(-time.Minute), now.Add(time.Hour))
	caFile, serverCertFile, serverKeyFile := writeTestTLSFiles(t, authority, serverCertificate, "server")
	_, clientCertFile, clientKeyFile := writeTestTLSFiles(t, authority, clientCertificate, "client")
	_, untrustedCertFile, untrustedKeyFile := writeTestTLSFiles(t, otherAuthority, untrustedClientCertificate, "intruder")

	serverConfig, err := NewServerConfig(ServerFiles{CertificateFile: serverCertFile, PrivateKeyFile: serverKeyFile, ClientCAFile: caFile}, MutualTLS)
	require.NoError(t, err)
	trustedClient, err := NewClientConfigLoader(ClientFiles{RootCAFile: caFile, CertificateFile: clientCertFile, PrivateKeyFile: clientKeyFile, ServerName: "control.internal"})
	require.NoError(t, err)
	trustedConfig, err := trustedClient.Load()
	require.NoError(t, err)
	require.NoError(t, tlsHandshake(serverConfig, trustedConfig))

	serverAuthOnlyClient, err := NewClientConfigLoader(ClientFiles{RootCAFile: caFile, ServerName: "control.internal"})
	require.NoError(t, err)
	serverAuthOnlyConfig, err := serverAuthOnlyClient.Load()
	require.NoError(t, err)
	require.Error(t, tlsHandshake(serverConfig, serverAuthOnlyConfig))

	untrustedClient, err := NewClientConfigLoader(ClientFiles{RootCAFile: caFile, CertificateFile: untrustedCertFile, PrivateKeyFile: untrustedKeyFile, ServerName: "control.internal"})
	require.NoError(t, err)
	untrustedConfig, err := untrustedClient.Load()
	require.NoError(t, err)
	require.Error(t, tlsHandshake(serverConfig, untrustedConfig))
}

func TestRuntimeTLSRejectsWrongTrustAndServerName(t *testing.T) {
	now := time.Now().UTC()
	authority := newTestAuthority(t, "runtime-ca", now)
	otherAuthority := newTestAuthority(t, "other-ca", now)
	serverCertificate := authority.issue(t, "runtime", []string{"runtime.internal"}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now.Add(-time.Minute), now.Add(time.Hour))
	caFile, serverCertFile, serverKeyFile := writeTestTLSFiles(t, authority, serverCertificate, "runtime")
	otherCAFile, _, _ := writeTestTLSFiles(t, otherAuthority, serverCertificate, "other")

	serverConfig, err := NewServerConfig(ServerFiles{CertificateFile: serverCertFile, PrivateKeyFile: serverKeyFile}, ServerAuthOnly)
	require.NoError(t, err)
	validLoader, err := NewClientConfigLoader(ClientFiles{RootCAFile: caFile, ServerName: "runtime.internal"})
	require.NoError(t, err)
	validConfig, err := validLoader.Load()
	require.NoError(t, err)
	require.NoError(t, tlsHandshake(serverConfig, validConfig))

	wrongNameLoader, err := NewClientConfigLoader(ClientFiles{RootCAFile: caFile, ServerName: "wrong.internal"})
	require.NoError(t, err)
	wrongNameConfig, err := wrongNameLoader.Load()
	require.NoError(t, err)
	require.Error(t, tlsHandshake(serverConfig, wrongNameConfig))

	wrongCALoader, err := NewClientConfigLoader(ClientFiles{RootCAFile: otherCAFile, ServerName: "runtime.internal"})
	require.NoError(t, err)
	wrongCAConfig, err := wrongCALoader.Load()
	require.NoError(t, err)
	require.Error(t, tlsHandshake(serverConfig, wrongCAConfig))
}

func TestTLSFilesRejectExpiredCertificate(t *testing.T) {
	now := time.Now().UTC()
	authority := newTestAuthority(t, "expired-ca", now)
	expired := authority.issue(t, "expired", []string{"expired.internal"}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now.Add(-2*time.Hour), now.Add(-time.Hour))
	_, certFile, keyFile := writeTestTLSFiles(t, authority, expired, "expired")

	_, err := NewServerConfig(ServerFiles{CertificateFile: certFile, PrivateKeyFile: keyFile}, ServerAuthOnly)
	require.Error(t, err)
}

func TestServerCertificateAndClientTrustRotateOnNewHandshake(t *testing.T) {
	now := time.Now().UTC()
	oldAuthority := newTestAuthority(t, "old-ca", now)
	newAuthority := newTestAuthority(t, "new-ca", now)
	oldCertificate := oldAuthority.issue(t, "runtime", []string{"runtime.internal"}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now.Add(-time.Minute), now.Add(time.Hour))
	newCertificate := newAuthority.issue(t, "runtime", []string{"runtime.internal"}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, now.Add(-time.Minute), now.Add(time.Hour))
	rootFile, certificateFile, keyFile := writeTestTLSFiles(t, oldAuthority, oldCertificate, "runtime")
	appendCertificate(t, rootFile, newAuthority.certificatePEM)

	serverConfig, err := NewServerConfig(ServerFiles{CertificateFile: certificateFile, PrivateKeyFile: keyFile}, ServerAuthOnly)
	require.NoError(t, err)
	clientLoader, err := NewClientConfigLoader(ClientFiles{RootCAFile: rootFile, ServerName: "runtime.internal"})
	require.NoError(t, err)
	clientConfig, err := clientLoader.Load()
	require.NoError(t, err)
	require.NoError(t, tlsHandshake(serverConfig, clientConfig))

	overwriteFile(t, certificateFile, newCertificate.certificatePEM)
	overwriteFile(t, keyFile, newCertificate.privateKeyPEM)
	clientConfig, err = clientLoader.Load()
	require.NoError(t, err)
	require.NoError(t, tlsHandshake(serverConfig, clientConfig))

	overwriteFile(t, rootFile, newAuthority.certificatePEM)
	clientConfig, err = clientLoader.Load()
	require.NoError(t, err)
	require.NoError(t, tlsHandshake(serverConfig, clientConfig))
	require.Equal(t, uint16(tls.VersionTLS13), clientConfig.MinVersion)
}
