package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// AdvisoryLockKey namespaces the process-wide locks this application takes.
// The zero value is deliberately not a key, so a forgotten one fails loudly
// rather than silently sharing a lock with the next thing that forgets.
//
// Postgres advisory locks are a single global int64 namespace per database.
// Every key an application uses belongs in this one block, so a collision is
// visible at a glance rather than discovered as a mutual deadlock in prod.
type AdvisoryLockKey int64

const (
	// AdvisoryLockMigration serialises schema migration across replicas.
	AdvisoryLockMigration AdvisoryLockKey = iota + 1
	// AdvisoryLockSeedAdmin AdvisoryLockKey  // add further keys here
)

// advisoryLockTimeout bounds the wait for a peer. Long enough for the real
// work to finish, short enough that a wedged holder surfaces as a failure
// rather than a process hanging forever with no output.
const advisoryLockTimeout = "60s"

// WithAdvisoryLock runs fn while holding key, excluding every other session
// that asks for the same key. The lock is session-scoped rather than
// transaction-scoped because fn may need to run its own transactions — goose
// does — and so cannot itself be wrapped in one.
//
// A holder that dies releases the lock when its backend terminates, so a
// crashed peer cannot wedge the lock permanently.
func WithAdvisoryLock(ctx context.Context, db *sql.DB, key AdvisoryLockKey, fn func(context.Context) error) error {
	// Pinned: pg_advisory_unlock only releases a lock taken on the same
	// session, and the pool would otherwise hand the release to a different
	// connection.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: pin connection for advisory lock: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SET lock_timeout = '"+advisoryLockTimeout+"'"); err != nil {
		return fmt.Errorf("store: set lock timeout: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", int64(key)); err != nil {
		return fmt.Errorf("store: acquire advisory lock %d: %w", key, err)
	}
	defer func() {
		// WithoutCancel: a cancelled boot must still release the lock now,
		// not whenever the backend eventually goes away.
		//nolint:errcheck // The lock also releases when this session ends.
		conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", int64(key))
	}()

	return fn(ctx)
}

// MigrateUp is the canonical caller: every replica runs this at boot, and
// goose is not safe to race — concurrent runs fight over the version table.
// The replica that arrives second waits, then finds nothing to do.
func MigrateUp(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("store.MigrateUp: open db: %w", err)
	}
	defer db.Close()

	err = WithAdvisoryLock(ctx, db, AdvisoryLockMigration, func(context.Context) error {
		return goose.Up(db, "migrations")
	})
	if err != nil {
		return fmt.Errorf("store.MigrateUp: run migrations: %w", err)
	}
	return nil
}

// WithAdvisoryXactLock is the transaction-scoped variant, for the case where
// the locked work IS a single transaction. The lock releases on commit or
// rollback with no unlock call and no pinned connection — strictly simpler
// than WithAdvisoryLock, so prefer it whenever it fits.
//
// It does not fit goose: goose runs its own transaction per migration.
func WithAdvisoryXactLock[T any](ctx context.Context, pool *pgxpool.Pool, q *Queries,
	key AdvisoryLockKey, fn TxFunc[T],
) (T, error) {
	return InTx(ctx, pool, q, func(ctx context.Context, tx pgx.Tx, q *Queries) (T, error) {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(key)); err != nil {
			var zero T
			return zero, fmt.Errorf("store: acquire advisory xact lock %d: %w", key, err)
		}
		return fn(ctx, tx, q)
	})
}
