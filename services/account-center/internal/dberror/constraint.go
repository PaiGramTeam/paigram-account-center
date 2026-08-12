package dberror

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const postgresUniqueViolation = "23505"

const postgresCheckViolation = "23514"

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

// IsCheckConstraint reports whether err is the named PostgreSQL CHECK violation.
func IsCheckConstraint(err error, constraintName string) bool {
	if err == nil || strings.TrimSpace(constraintName) == "" {
		return false
	}
	var postgresErr *pgconn.PgError
	return errors.As(err, &postgresErr) &&
		postgresErr.Code == postgresCheckViolation &&
		postgresErr.ConstraintName == constraintName
}
