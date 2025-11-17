package store

import "context"

// TransactionManager coordinates repository work inside a single transaction.
type TransactionManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Provider returns repository-backed transaction managers.
type Provider interface {
	TransactionManager() TransactionManager
}

// NopTransactionManager executes callbacks immediately without persistence.
type NopTransactionManager struct{}

var _ TransactionManager = (*NopTransactionManager)(nil)

func (n *NopTransactionManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx)
}
