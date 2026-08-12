package data

import (
	"context"

	"gorm.io/gorm"
)

type txContextKey struct{}

func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func dbFromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx := txFromContext(ctx); tx != nil {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}

func txFromContext(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(txContextKey{}).(*gorm.DB)
	return tx
}
