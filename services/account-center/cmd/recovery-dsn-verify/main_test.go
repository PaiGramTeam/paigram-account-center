package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestPGXParsesEncodedTargetOverrides(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://user:password@postgres/paigram?h%6fst=remote&data%62ase=other")
	require.NoError(t, err)
	require.Equal(t, "remote", config.ConnConfig.Host)
	require.Equal(t, "other", config.ConnConfig.Database)
	require.Error(t, validateDSN("postgres://user:password@postgres/paigram?h%6fst=remote&data%62ase=other", "paigram"))
}

func TestValidateDSNAcceptsExactRestoredTarget(t *testing.T) {
	require.NoError(t, validateDSN("postgres://user:password@postgres:5432/paigram?sslmode=disable\n", "paigram"))
}

func TestValidateDSNRejectsRemoteFallbacks(t *testing.T) {
	testCases := []string{
		"postgres://user:password@postgres:5432,remote:5432/paigram?sslmode=disable",
		"postgres://user:password@ignored/paigram?host=postgres,remote&port=5432,5432&sslmode=disable",
	}
	for _, dsn := range testCases {
		require.Error(t, validateDSN(dsn, "paigram"), dsn)
	}
}
