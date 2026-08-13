package certificateexpiry

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

// ReadNotAfter returns the earliest expiry in a PEM certificate or trust bundle.
func ReadNotAfter(path string) (time.Time, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("read certificate: %w", err)
	}
	var earliest time.Time
	for len(raw) > 0 {
		block, remainder := pem.Decode(raw)
		if block == nil {
			break
		}
		raw = remainder
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			return time.Time{}, fmt.Errorf("parse certificate: %w", parseErr)
		}
		if earliest.IsZero() || certificate.NotAfter.Before(earliest) {
			earliest = certificate.NotAfter
		}
	}
	if earliest.IsZero() {
		return time.Time{}, errors.New("certificate file does not contain a PEM certificate")
	}
	return earliest.UTC(), nil
}
