package transporttls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const maximumPEMFileSize = 1024 * 1024

var ErrInvalidTLSFiles = errors.New("invalid TLS file configuration")

type ServerMode uint8

const (
	ServerAuthOnly ServerMode = iota + 1
	MutualTLS
)

type ServerFiles struct {
	CertificateFile string
	PrivateKeyFile  string
	ClientCAFile    string
}

type ClientFiles struct {
	RootCAFile      string
	CertificateFile string
	PrivateKeyFile  string
	ServerName      string
}

type ClientConfigLoader struct {
	files ClientFiles
}

func NewServerConfig(files ServerFiles, mode ServerMode) (*tls.Config, error) {
	if err := ValidateServerFiles(files, mode); err != nil {
		return nil, err
	}
	config, err := loadServerConfig(files, mode)
	if err != nil {
		return nil, err
	}
	config.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		return loadServerConfig(files, mode)
	}
	return config, nil
}

func NewClientConfigLoader(files ClientFiles) (*ClientConfigLoader, error) {
	if strings.TrimSpace(files.ServerName) == "" {
		return nil, fmt.Errorf("%w: server name is required", ErrInvalidTLSFiles)
	}
	if err := ValidateClientFiles(files, false); err != nil {
		return nil, err
	}
	loader := &ClientConfigLoader{files: files}
	return loader, nil
}

func ValidateClientFiles(files ClientFiles, requireCertificate bool) error {
	if strings.TrimSpace(files.RootCAFile) == "" {
		return fmt.Errorf("%w: root CA file is required", ErrInvalidTLSFiles)
	}
	hasCertificate := strings.TrimSpace(files.CertificateFile) != ""
	hasPrivateKey := strings.TrimSpace(files.PrivateKeyFile) != ""
	if hasCertificate != hasPrivateKey {
		return fmt.Errorf("%w: client certificate and private key files must be configured together", ErrInvalidTLSFiles)
	}
	if requireCertificate && !hasCertificate {
		return fmt.Errorf("%w: client certificate and private key files are required", ErrInvalidTLSFiles)
	}
	if _, err := loadCAPool(files.RootCAFile, time.Now().UTC()); err != nil {
		return err
	}
	if hasCertificate {
		if _, err := loadCertificate(files.CertificateFile, files.PrivateKeyFile, x509.ExtKeyUsageClientAuth, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func (l *ClientConfigLoader) Load() (*tls.Config, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: client config loader is nil", ErrInvalidTLSFiles)
	}
	roots, err := loadCAPool(l.files.RootCAFile, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	config := baseTLSConfig()
	config.RootCAs = roots
	config.ServerName = strings.TrimSpace(l.files.ServerName)
	if strings.TrimSpace(l.files.CertificateFile) != "" {
		certificate, err := loadCertificate(l.files.CertificateFile, l.files.PrivateKeyFile, x509.ExtKeyUsageClientAuth, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	return config, nil
}

func ValidateServerFiles(files ServerFiles, mode ServerMode) error {
	if strings.TrimSpace(files.CertificateFile) == "" || strings.TrimSpace(files.PrivateKeyFile) == "" {
		return fmt.Errorf("%w: server certificate and private key files are required", ErrInvalidTLSFiles)
	}
	if mode != ServerAuthOnly && mode != MutualTLS {
		return fmt.Errorf("%w: unsupported server mode", ErrInvalidTLSFiles)
	}
	if mode == MutualTLS && strings.TrimSpace(files.ClientCAFile) == "" {
		return fmt.Errorf("%w: client CA file is required for mutual TLS", ErrInvalidTLSFiles)
	}
	return nil
}

func loadServerConfig(files ServerFiles, mode ServerMode) (*tls.Config, error) {
	now := time.Now().UTC()
	certificate, err := loadCertificate(files.CertificateFile, files.PrivateKeyFile, x509.ExtKeyUsageServerAuth, now)
	if err != nil {
		return nil, err
	}
	config := baseTLSConfig()
	config.Certificates = []tls.Certificate{certificate}
	config.SessionTicketsDisabled = true
	if mode == MutualTLS {
		clientCAs, err := loadCAPool(files.ClientCAFile, now)
		if err != nil {
			return nil, err
		}
		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.ClientCAs = clientCAs
	}
	return config, nil
}

func baseTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"h2"},
	}
}

func loadCertificate(certificateFile, privateKeyFile string, usage x509.ExtKeyUsage, now time.Time) (tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(certificateFile, privateKeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: load certificate pair: %w", ErrInvalidTLSFiles, err)
	}
	if len(certificate.Certificate) == 0 {
		return tls.Certificate{}, fmt.Errorf("%w: certificate chain is empty", ErrInvalidTLSFiles)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: parse leaf certificate: %w", ErrInvalidTLSFiles, err)
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return tls.Certificate{}, fmt.Errorf("%w: certificate is not currently valid", ErrInvalidTLSFiles)
	}
	if !supportsUsage(leaf, usage) {
		return tls.Certificate{}, fmt.Errorf("%w: certificate has the wrong extended key usage", ErrInvalidTLSFiles)
	}
	certificate.Leaf = leaf
	return certificate, nil
}

func loadCAPool(path string, now time.Time) (*x509.CertPool, error) {
	raw, err := readBoundedFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	remaining := raw
	validAuthorities := 0
	for len(strings.TrimSpace(string(remaining))) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("%w: CA file contains invalid PEM", ErrInvalidTLSFiles)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: parse CA certificate: %w", ErrInvalidTLSFiles, err)
		}
		if !certificate.IsCA {
			return nil, fmt.Errorf("%w: trust anchor is not a CA", ErrInvalidTLSFiles)
		}
		if !now.Before(certificate.NotBefore) && now.Before(certificate.NotAfter) {
			pool.AddCert(certificate)
			validAuthorities++
		}
		remaining = rest
	}
	if validAuthorities == 0 {
		return nil, fmt.Errorf("%w: CA file contains no currently valid authority", ErrInvalidTLSFiles)
	}
	return pool, nil
}

func readBoundedFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect PEM file: %w", ErrInvalidTLSFiles, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumPEMFileSize {
		return nil, fmt.Errorf("%w: PEM file must be a non-empty regular file no larger than %d bytes", ErrInvalidTLSFiles, maximumPEMFileSize)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read PEM file: %w", ErrInvalidTLSFiles, err)
	}
	return raw, nil
}

func supportsUsage(certificate *x509.Certificate, expected x509.ExtKeyUsage) bool {
	for _, usage := range certificate.ExtKeyUsage {
		if usage == expected || usage == x509.ExtKeyUsageAny {
			return true
		}
	}
	return false
}
