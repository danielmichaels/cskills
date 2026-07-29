---
name: pgx-transactions
category: custom
description: Transactions and advisory locks in Go with pgx and sqlc — the house helpers (InTx / WithTx / EnsureTx / WithAdvisoryLock) that replace inline pool.Begin/Commit/Rollback. Use this skill whenever code opens a database transaction, writes more than one row atomically, enqueues a job alongside a write, or takes a Postgres lock. Trigger on "pool.Begin", "tx.Commit", "tx.Rollback", "BeginFunc", "BeginTx", "WithTx", "qtx", "transaction", "atomic write", "rollback", "savepoint", "advisory lock", "pg_advisory_lock", "pg_advisory_xact_lock", "lock_timeout", "serialize migrations", "two replicas racing", "sqlc WithTx", or any function taking a pgx.Tx parameter. Also use when reviewing a service method that writes two or more tables, when a write reports success but the row is missing, or when the connection pool is exhausted.
---

# Transactions with pgx + sqlc

Service code should never contain `pool.Begin`, `tx.Commit`, or `tx.Rollback`.
Those belong in exactly one file, wrapped in callback helpers. This skill is
the shape of those helpers, the traps in writing them, and the review rules
for the call sites.

## When this skill applies

- Any Go service writing to Postgres through `pgx/v5` + sqlc-generated code.
- Any write spanning two or more statements that must land together.
- Enqueuing a background job in the same transaction as the write it depends
  on (see also the `river` skill — this is *how* it gets its `tx`).
- Serialising work across replicas: migrations, seeds, singleton jobs.

## The one thing to internalize first

**A forgotten `Commit` is silent.** The idiomatic inline shape is:

```go
tx, err := pool.Begin(ctx)
defer tx.Rollback(ctx) //nolint:errcheck
// ... writes ...
tx.Commit(ctx)
```

Delete that last line and nothing breaks. No error, no panic, no failing
build. The `defer` rolls the work back and the function returns `nil` — a
write that reports success and never happened, discovered weeks later as
missing data. A callback helper makes this failure mode unrepresentable,
because the helper owns the commit and the call site cannot forget it.

That is the entire argument. Everything below is how to build it correctly.

## Core principles (the non-negotiables)

### 1. Build on `pgx.BeginFunc`, do not hand-roll `defer`/`recover`

pgx v5 ships `pgx.BeginFunc(ctx, db, fn)`, which begins, commits on nil, rolls
back on error, and — unlike most hand-rolled versions — **reports a rollback
error instead of dropping it**. It needs no `recover()`: a panic still runs the
deferred `Rollback` and then propagates, which is the behaviour the `recover`/
`panic` dance was written to get.

Hand-rolling is the `database/sql` pattern. `database/sql` has no `BeginFunc`;
pgx does. Do not port the older shape into a pgx codebase.

### 2. The named-return trap kills commits silently

The classic hand-rolled helper:

```go
func WithTransaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
    tx, err := db.BeginTx(ctx, nil)
    // ...
    defer func() {
        if err != nil { _ = tx.Rollback() } else { err = tx.Commit() }  // <-- BUG
    }()
    err = fn(tx)
    return err
}
```

The return type is a plain `error`, not `(err error)`. The deferred
`err = tx.Commit()` assigns to a **local** that nobody reads, so **a failed
commit is reported as success**. The version with `(err error)` works; the one
without is broken, and the two look identical at a glance.

This bug travels in pairs — a codebase with `WithTransaction` (named return,
correct) beside `WithAdvisoryLockTxn` (unnamed, broken) is common, because the
second was copy-pasted and the signature was retyped. **Whenever you see a
deferred commit, check the return signature first.**

Principle 1 removes the trap entirely: `pgx.BeginFunc` does the commit, so
your helper never writes a deferred assignment.

### 3. On error, return the zero value — never a partial result

```go
if err != nil {
    var zero T
    return zero, err   // NOT the value the closure computed
}
```

Assigning the closure's result before checking the error leaks a populated
struct out of a transaction that rolled back. A caller that ignores the error —
and some will — then acts on a row that does not exist.

### 4. The callback takes `(ctx, tx pgx.Tx, q *Queries)`

`q` is already bound to the transaction, so **no call site ever writes
`q.WithTx(tx)` again**. `tx` is still passed because some APIs need the raw
handle — River's `InsertTx` above all. Passing only one of the two guarantees
boilerplate at every site that needs the other.

### 5. `EnsureTx` joins, it does not nest

A function that may or may not be called mid-transaction takes `*Queries`, not
`pgx.Tx`, and routes through `EnsureTx`. If `q` is already transaction-bound it
runs inline on that transaction; otherwise it opens its own.

`pgx.Tx.Begin()` *does* offer real nesting via savepoints — and it is almost
always the wrong tool here. A savepoint that commits while the outer
transaction rolls back still vanishes, so the nesting buys nothing but the
illusion of independence. Join.

This is what lets `RecordInBurst(ctx, q, ...)` be called both standalone and as
one step of a bigger write, with no flag and no `tx` the caller cannot supply.

### 6. Do not wrap errors inside the helper

Return the callback's error unwrapped. Call sites already wrap with their own
package prefix (`fmt.Errorf("boards: create: %w", err)`), and sentinel matching
(`errors.Is(err, store.ErrNotFound)`) must survive the trip. A helper that adds
`"tx: "` to everything produces doubled prefixes and nothing else.

### 7. Never hold a transaction open across network I/O

Upload to object storage, call the payment API, resize the image — **then**
open the transaction. A transaction pinned across a dozen slow uploads holds a
pool connection the whole time and starves everything else.

The shape is: stage all the slow work first, open the transaction, write, close
it. Staged artifacts that never get recorded are cleaned up separately and
best-effort — object storage has no rollback, so this asymmetry is real and
must be designed for, not wished away.

### 8. Inside a transaction, any error aborts everything

Postgres is not MySQL here. A unique-violation on statement 3 poisons the
transaction: statements 4+ all fail with `current transaction is aborted`, even
if you handled the violation in Go. So the get-or-create pattern **cannot** be
`INSERT` → catch violation → `SELECT`.

Use insert-if-absent plus re-read:

```sql
INSERT INTO users (email) VALUES ($1) ON CONFLICT (email) DO NOTHING;
SELECT * FROM users WHERE email = $1;   -- always returns, whoever won
```

Without `ON CONFLICT DO NOTHING`, a concurrent writer racing on the same email
kills the whole transaction rather than one statement.

## Advisory locks — pick the right scope

| Need | Use |
|---|---|
| The locked work *is* one transaction | `pg_advisory_xact_lock` — releases on commit/rollback, no unlock call, no pinned conn |
| The locked work runs its own transactions (goose, multi-step jobs) | `pg_advisory_lock` on a **pinned connection**, explicit unlock |
| Non-blocking "someone else has it, skip" | `pg_try_advisory_lock` |

**Transaction-scoped is strictly simpler — prefer it whenever it fits.** But it
does not fit goose: goose runs its own transaction per migration and cannot be
wrapped in one. That is the single most common mis-port of this pattern.

Session-scoped rules:

- **Pin the connection** with `db.Conn(ctx)`. `pg_advisory_unlock` only
  releases a lock taken on the *same session*; a pooled `db.ExecContext` may
  land on a different connection and silently fail to release.
- **Set `lock_timeout`.** `pg_advisory_lock` blocks forever by default, so a
  wedged peer turns into a boot that hangs with no output. A bounded wait fails
  loudly instead.
- **Unlock with `context.WithoutCancel`.** If the boot context is cancelled
  mid-work, an unlock on that context is refused before it reaches Postgres.
- **Keys start at 1, not `iota` from 0.** The zero value must not be a valid
  key, or a forgotten one silently shares a lock with the next thing that
  forgets. Keep every key in one `const` block — advisory locks are a single
  global `int64` namespace per database, so collisions are invisible until they
  deadlock.

A crashed holder always releases: the lock dies with its backend.

## Review mode — what to look for

- **`pool.Begin` anywhere outside the helper file.** The whole rule.
- **A deferred commit with an unnamed `error` return** → principle 2.
- **`tx.Commit` in a different function from `tx.Begin`.** The commit-in-a-
  callee smell: nothing at the `Begin` line says where the transaction ends.
- **A helper assigning its result before checking the error** → principle 3.
- **A function taking `tx pgx.Tx` that is only ever called from one place** →
  candidate for `*Queries` + `EnsureTx`.
- **`s3.Put` / `http.Do` / `exec.Command` between a `Begin` and a `Commit`** →
  principle 7.
- **`INSERT` with error-catching instead of `ON CONFLICT DO NOTHING`** inside a
  transaction → principle 8.
- **`pg_advisory_xact_lock` around anything that manages its own transactions.**
- **A job enqueued outside the transaction of the write it depends on** — see
  the `river` skill, principle 1.

## Debug mode — symptom → cause

| Symptom | Cause |
|---|---|
| Write returns nil, row is missing | Forgotten `Commit` under `defer Rollback`. Grep for `Begin` without a matching `Commit` in the same function. |
| Commit errors are never observed in logs | Deferred `err = tx.Commit()` with an unnamed return (principle 2). |
| Caller acts on a struct from a rolled-back tx | Result assigned before the error check (principle 3). |
| `current transaction is aborted, commands ignored` | An earlier statement errored; Postgres poisoned the tx (principle 8). |
| Pool exhausted under moderate load | Transaction held across network I/O (principle 7), or a leaked `Begin` with no close path. |
| Boot hangs forever, no log output | `pg_advisory_lock` with no `lock_timeout`, waiting on a peer. |
| Advisory lock never releases | Unlock ran on a different pooled connection, or on a cancelled context. |
| Two replicas both run migrations, one errors | No advisory lock around `goose.Up`. |
| Work inside a "nested" tx vanishes | Savepoint nesting under an outer rollback — join instead (principle 5). |

## Testing the helpers

These are the tests that catch the bugs above. Full versions in
`references/tx_test.go`.

- **Commits** — row visible on the pool afterwards.
- **Rolls back on error** — row absent, and the callback's error comes back
  matchable with `errors.Is` (proves principle 6).
- **Rolls back on panic and re-panics** — asserts both the recovered value and
  the absent row. Catches a hand-rolled helper with a broken recover path.
- **Zero value on error** — catches principle 3.
- **`EnsureTx` joins** — run it against a `q` bound to an outer tx, roll the
  outer tx back, assert the inner write vanished. This is the only way to prove
  it joined rather than opened its own.
- **`EnsureTx` opens** — pool-bound `q`, both commit and rollback paths.
- **Advisory lock excludes and releases** — hold it, assert
  `pg_try_advisory_lock` **from a second `sql.DB`** fails, then assert it
  succeeds after release. Asking on the same handle proves nothing: advisory
  locks are re-entrant within the session that holds them.

Converting existing inline transactions? **The existing service tests are the
regression net.** Behaviour must not change, so they should pass untouched —
especially any "the job insert failed, so nothing was written" test, which is
precisely a rollback assertion. If one needs editing, the conversion moved a
transaction boundary and is wrong.

Use the `embedded-postgres` skill for the test database. Do not mock the DB
layer for this — the entire subject is real transactional semantics.

## Reference files

- `references/tx.go` — `InTx` / `WithTx` / `EnsureTx`. Copy into the package
  that owns your sqlc output; `EnsureTx` type-asserts the unexported `db` field
  on `Queries` and can only compile there.
- `references/advisory-lock.go` — `AdvisoryLockKey`, session-scoped
  `WithAdvisoryLock` with the goose caller, plus the transaction-scoped variant.
- `references/tx_test.go` — the full suite described above.

## Style notes for using this skill

- The helpers live in the sqlc package (`internal/store/tx.go`), not a generic
  `dbutil` — `EnsureTx` cannot work anywhere else.
- Converting a codebase: do the helper and its tests first, then convert call
  sites one package at a time, running that package's existing tests after
  each. A clean conversion touches no test file.
- Verify completeness with a grep, not by memory:
  `grep -rn --include='*.go' -E '\.Begin\(|tx\.Commit\(|tx\.Rollback\('` should
  return the helper file and deliberate raw-transaction tests, nothing else.
- Expect the conversion to *remove* lines. Roughly six lines of ceremony per
  site go away, along with every `"begin tx: %w"` and `"commit: %w"` wrap.
