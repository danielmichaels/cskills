// Package store — transaction helpers. Copy into the package that owns your
// sqlc output (the generated Queries type), because EnsureTx type-asserts the
// unexported db field and can only compile there.
package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxFunc is the body of a transaction. It receives Queries already bound to
// the transaction, plus the raw tx for the things that need one directly —
// River's InsertTx is usually the only such caller.
type TxFunc[T any] func(ctx context.Context, tx pgx.Tx, q *Queries) (T, error)

// InTx runs fn in a transaction, committing when it returns nil and rolling
// back otherwise. A panic rolls back and propagates. fn's error is returned
// unwrapped so callers keep matching their own sentinels through it; on any
// failure the returned T is the zero value, never a half-built result from a
// transaction that did not commit.
func InTx[T any](ctx context.Context, pool *pgxpool.Pool, q *Queries, fn TxFunc[T]) (T, error) {
	var out T
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		v, err := fn(ctx, tx, q.WithTx(tx))
		if err != nil {
			return err
		}
		out = v
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return out, nil
}

// WithTx is InTx for a transaction that produces no value.
func WithTx(ctx context.Context, pool *pgxpool.Pool, q *Queries,
	fn func(ctx context.Context, tx pgx.Tx, q *Queries) error,
) error {
	_, err := InTx(ctx, pool, q,
		func(ctx context.Context, tx pgx.Tx, q *Queries) (struct{}, error) {
			return struct{}{}, fn(ctx, tx, q)
		})
	return err
}

// EnsureTx runs fn in q's transaction when q is already bound to one, and in
// a new transaction otherwise. Joining rather than nesting is deliberate: the
// caller's commit or rollback decides fn's fate too, so a helper that may or
// may not be called mid-transaction cannot half-commit its part.
//
// This is what lets a function like RecordInBurst be called both standalone
// and as one step of a larger write, without the caller passing a flag or the
// function taking a pgx.Tx it cannot supply.
func EnsureTx[T any](ctx context.Context, pool *pgxpool.Pool, q *Queries, fn TxFunc[T]) (T, error) {
	if tx, ok := q.db.(pgx.Tx); ok {
		return fn(ctx, tx, q)
	}
	return InTx(ctx, pool, q, fn)
}
