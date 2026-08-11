package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectRejectsMalformedPostgreSQLDSN(t *testing.T) {
	_, err := Connect(Config{DSN: "mysql://root:password@127.0.0.1/platform"})

	require.ErrorContains(t, err, "parse PostgreSQL DSN")
}

func TestConnectRejectsMissingDSN(t *testing.T) {
	_, err := Connect(Config{})

	require.EqualError(t, err, "database DSN is required")
}
