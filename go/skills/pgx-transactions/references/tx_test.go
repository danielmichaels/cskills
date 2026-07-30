// External test package (store_test, not store): the helpers are exercised
// through the same surface services use, and it breaks the import cycle with
// a testhelpers package that itself imports store.
package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/matryer/is"

	"yourmod/internal/store"
	"yourmod/internal/testhelpers"
)

var errBoom = errors.New("boom")

func newUser(email string) store.CreateUserParams {
	return store.CreateUserParams{Email: email, DisplayName: "Tx"}
}

func TestWithTxCommits(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	err := store.WithTx(ctx, pg.Pool, pg.Queries,
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) error {
			_, err := q.CreateUser(ctx, newUser("commits@example.com"))
			return err
		})
	is.NoErr(err)

	_, err = pg.Queries.GetUserByEmail(ctx, "commits@example.com")
	is.NoErr(err)
}

func TestWithTxRollsBackOnError(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	err := store.WithTx(ctx, pg.Pool, pg.Queries,
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) error {
			if _, err := q.CreateUser(ctx, newUser("rollback@example.com")); err != nil {
				return err
			}
			return errBoom
		})
	// The callback's error reaches the caller unwrapped, so services can keep
	// matching their own sentinels through the helper.
	is.True(errors.Is(err, errBoom))

	_, err = pg.Queries.GetUserByEmail(ctx, "rollback@example.com")
	is.True(errors.Is(err, pgx.ErrNoRows))
}

// The test that catches a hand-rolled helper missing its recover path.
func TestWithTxRollsBackOnPanicAndRepanics(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	func() {
		defer func() {
			is.Equal(recover(), "callback exploded")
		}()
		_ = store.WithTx(ctx, pg.Pool, pg.Queries,
			func(ctx context.Context, _ pgx.Tx, q *store.Queries) error {
				if _, err := q.CreateUser(ctx, newUser("panic@example.com")); err != nil {
					return err
				}
				panic("callback exploded")
			})
		t.Fatal("panic did not propagate out of WithTx")
	}()

	_, err := pg.Queries.GetUserByEmail(ctx, "panic@example.com")
	is.True(errors.Is(err, pgx.ErrNoRows))
}

// The test that catches `out = v` being assigned before the error check.
func TestInTxReturnsZeroValueOnError(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	got, err := store.InTx(ctx, pg.Pool, pg.Queries,
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) (store.Users, error) {
			u, err := q.CreateUser(ctx, newUser("zero@example.com"))
			if err != nil {
				return store.Users{}, err
			}
			// A populated value alongside an error must not escape.
			return u, errBoom
		})
	is.True(errors.Is(err, errBoom))
	is.Equal(got, store.Users{})
}

func TestInTxCommitsAndReturnsValue(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	got, err := store.InTx(ctx, pg.Pool, pg.Queries,
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) (store.Users, error) {
			return q.CreateUser(ctx, newUser("value@example.com"))
		})
	is.NoErr(err)
	is.Equal(got.Email, "value@example.com")

	persisted, err := pg.Queries.GetUserByEmail(ctx, "value@example.com")
	is.NoErr(err)
	is.Equal(persisted.ID, got.ID)
}

// Proves EnsureTx joined rather than opened: rolling back the caller's
// transaction must discard the callback's write too.
func TestEnsureTxJoinsCallersTransaction(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	outer, err := pg.Pool.Begin(ctx)
	is.NoErr(err)

	_, err = store.EnsureTx(ctx, pg.Pool, pg.Queries.WithTx(outer),
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) (store.Users, error) {
			return q.CreateUser(ctx, newUser("joined@example.com"))
		})
	is.NoErr(err)

	is.NoErr(outer.Rollback(ctx))

	_, err = pg.Queries.GetUserByEmail(ctx, "joined@example.com")
	is.True(errors.Is(err, pgx.ErrNoRows))
}

func TestEnsureTxOpensItsOwnTransaction(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)
	pg.TruncateAll(ctx, t)

	_, err := store.EnsureTx(ctx, pg.Pool, pg.Queries,
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) (store.Users, error) {
			return q.CreateUser(ctx, newUser("own@example.com"))
		})
	is.NoErr(err)

	_, err = pg.Queries.GetUserByEmail(ctx, "own@example.com")
	is.NoErr(err)

	_, err = store.EnsureTx(ctx, pg.Pool, pg.Queries,
		func(ctx context.Context, _ pgx.Tx, q *store.Queries) (store.Users, error) {
			if _, err := q.CreateUser(ctx, newUser("own-rollback@example.com")); err != nil {
				return store.Users{}, err
			}
			return store.Users{}, errBoom
		})
	is.True(errors.Is(err, errBoom))

	_, err = pg.Queries.GetUserByEmail(ctx, "own-rollback@example.com")
	is.True(errors.Is(err, pgx.ErrNoRows))
}

func TestWithAdvisoryLockExcludesOtherSessions(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)

	db, err := sql.Open("pgx", pg.DSN)
	is.NoErr(err)
	defer db.Close()

	contender, err := sql.Open("pgx", pg.DSN)
	is.NoErr(err)
	defer contender.Close()

	ran := false
	err = store.WithAdvisoryLock(ctx, db, store.AdvisoryLockMigration, func(context.Context) error {
		ran = true
		is.Equal(tryAdvisoryLock(ctx, is, contender), false)
		return nil
	})
	is.NoErr(err)
	is.True(ran)

	// The lock is session-scoped, so it must be released by the time
	// WithAdvisoryLock returns rather than when the pool recycles the conn.
	is.Equal(tryAdvisoryLock(ctx, is, contender), true)
}

func TestWithAdvisoryLockReleasesOnError(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	pg := testhelpers.Shared(ctx, t)

	db, err := sql.Open("pgx", pg.DSN)
	is.NoErr(err)
	defer db.Close()

	contender, err := sql.Open("pgx", pg.DSN)
	is.NoErr(err)
	defer contender.Close()

	err = store.WithAdvisoryLock(ctx, db, store.AdvisoryLockMigration, func(context.Context) error {
		return errBoom
	})
	is.True(errors.Is(err, errBoom))

	// A failed run must not strand the lock and wedge every later boot. Asked
	// from a different session, because advisory locks are re-entrant within
	// the session that holds them — asking on `db` would pass either way.
	is.Equal(tryAdvisoryLock(ctx, is, contender), true)
}

// tryAdvisoryLock reports whether db's session can take the migration lock,
// releasing it again when it can.
func tryAdvisoryLock(ctx context.Context, is *is.I, db *sql.DB) bool {
	is.Helper()
	var acquired bool
	err := db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)",
		int64(store.AdvisoryLockMigration)).Scan(&acquired)
	is.NoErr(err)
	if acquired {
		_, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock($1)",
			int64(store.AdvisoryLockMigration))
		is.NoErr(err)
	}
	return acquired
}
