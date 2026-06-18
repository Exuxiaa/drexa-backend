package repository

import (
	"context"

	"gorm.io/gorm"

	"drexa/internal/wallet"
)

// txKey is the context key under which an in-flight *gorm.DB transaction is stored.
type txKey struct{}

// dbFromContext returns the transaction-scoped handle if the context carries one (i.e. the call
// happens inside TxManager.Do), otherwise the repository's base handle. Either way the result is
// bound to ctx so cancellation and deadlines propagate.
func dbFromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}

type txManager struct {
	db *gorm.DB
}

// NewTxManager builds a wallet.TxManager backed by the given GORM handle.
func NewTxManager(db *gorm.DB) wallet.TxManager {
	return &txManager{db: db}
}

func (m *txManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	// Already inside a transaction — reuse it so nested calls share one atomic unit.
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return fn(ctx)
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}
