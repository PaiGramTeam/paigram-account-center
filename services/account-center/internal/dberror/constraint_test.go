package dberror

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "GORM", err: gorm.ErrDuplicatedKey, want: true},
		{name: "PostgreSQL", err: fmt.Errorf("insert user: %w", &pgconn.PgError{Code: postgresUniqueViolation}), want: true},
		{name: "SQLite", err: errors.New("UNIQUE constraint failed: users.email"), want: true},
		{name: "unrelated", err: errors.New("record is not unique enough to classify"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, IsUniqueViolation(test.err))
		})
	}
}

func TestIsCheckConstraint(t *testing.T) {
	err := fmt.Errorf("commit transaction: %w", &pgconn.PgError{
		Code:           postgresCheckViolation,
		ConstraintName: "active_administrator_required",
	})
	require.True(t, IsCheckConstraint(err, "active_administrator_required"))
	require.False(t, IsCheckConstraint(err, "different_constraint"))
	require.False(t, IsCheckConstraint(errors.New("check failed"), "active_administrator_required"))
}
