package certificateexpiry

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadNotAfterReadsLeafCertificate(t *testing.T) {
	bundle := tlstest.New(t, "certificate-expiry.internal")

	notAfter, err := ReadNotAfter(bundle.ServerCertFile)

	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC().Add(24*time.Hour), notAfter, time.Minute)
}

func TestReadNotAfterRejectsNonCertificatePEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-certificate.pem")
	require.NoError(t, os.WriteFile(path, []byte("-----BEGIN PRIVATE KEY-----\nAA==\n-----END PRIVATE KEY-----\n"), 0o600))

	_, err := ReadNotAfter(path)

	require.Error(t, err)
}
