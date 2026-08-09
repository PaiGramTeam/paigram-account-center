package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"paigram/internal/config"
)

func TestValidateConfigRequiresPostgresDSN(t *testing.T) {
	require.Error(t, validateConfig(config.DatabaseConfig{}))
	require.NoError(t, validateConfig(config.DatabaseConfig{
		DSN: "postgres://paigram:secret@localhost:5432/paigram?sslmode=disable",
	}))
}
