package platformbinding

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const grantTransactionAttempts = 4

type GrantTransactionObserver interface {
	ObserveGrantTransactionAttempt(error)
}

func runGrantTransaction(db *gorm.DB, observer GrantTransactionObserver, mutation func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < grantTransactionAttempts; attempt++ {
		err = db.Transaction(mutation, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if observer != nil {
			observer.ObserveGrantTransactionAttempt(err)
		}
		if !isRetryableGrantError(err) {
			return err
		}
	}
	return err
}

func isRetryableGrantError(err error) bool {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return false
	}
	switch postgresError.Code {
	case "40001", "40P01":
		return true
	case "23505":
		return postgresError.ConstraintName == "uk_consumer_grants_binding_consumer"
	default:
		return false
	}
}
