package dberror

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const postgresUniqueViolation = "23505"

// IsUniqueViolation reports whether err represents a database UNIQUE violation.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) {
		return postgresErr.Code == postgresUniqueViolation
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
