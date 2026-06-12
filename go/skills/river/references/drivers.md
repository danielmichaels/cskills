# River Drivers — Setup & Trade-offs (v0.39.0)

River talks to the database through a `riverdriver.Driver[TTx]`. The driver decides the transaction type `TTx`, whether `LISTEN/NOTIFY` is available, and how inserts are batched. Three drivers exist; pick with the decision tree in SKILL.md.

| | `riverpgxv5` | `riversqlite` | `riverdatabasesql` |
|---|---|---|---|
| Database | Postgres | SQLite (or libSQL) | Postgres |
| Underlying handle | `*pgxpool.Pool` | `*sql.DB` | `*sql.DB` |
| `TTx` (client type param) | `pgx.Tx` | `*sql.Tx` | `*sql.Tx` |
| LISTEN/NOTIFY | ✅ yes | ❌ no (polling only) | ❌ no (polling only) |
| `PollOnly` required | no | **yes** (effectively) | **yes** |
| Batched InsertMany | ✅ | ❌ one row at a time | partial |
| Maturity | production | newer, in active use (v0.39) | stable |
| Use when | default; concurrency, multi-process | single-node, embedded, dev, CLI | already on database/sql + ORM |

## riverpgxv5 (Postgres, default)

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/riverqueue/river"
    "github.com/riverqueue/river/riverdriver/riverpgxv5"
)

pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
if err != nil { return err }

client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
    Workers: workers,
})
```

- `riverpgxv5.New(pool *pgxpool.Pool) *Driver`. The pool may be **nil** for an insert-only-via-tx client (can `InsertTx` against a caller-supplied tx, but not `Start` or `Insert`).
- Client type is `*river.Client[pgx.Tx]`; transactional inserts take a `pgx.Tx` (from `pool.Begin(ctx)` or `pool.BeginTx`).
- LISTEN/NOTIFY gives near-instant job pickup. Don't set `PollOnly` unless you specifically want polling.
- Size `pool` for your worker concurrency: it must hold at least `sum(MaxWorkers)` plus headroom for inserts and River's own maintenance connections.

## riversqlite (SQLite / libSQL)

```go
import (
    "database/sql"
    _ "modernc.org/sqlite"   // or mattn/go-sqlite3 (CGo) — you choose; the driver wraps *sql.DB
    "github.com/riverqueue/river"
    "github.com/riverqueue/river/riverdriver/riversqlite"
)

db, err := sql.Open("sqlite", "file:app.db?_journal_mode=WAL&_busy_timeout=5000")
if err != nil { return err }
db.SetMaxOpenConns(1) // REQUIRED — see caveats

client, err := river.NewClient(riversqlite.New(db), &river.Config{
    Queues:            map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 5}},
    Workers:           workers,
    PollOnly:          true,                   // no LISTEN/NOTIFY on SQLite
    FetchPollInterval: 250 * time.Millisecond, // lower default-1s latency if you want snappier pickup
})
```

`riversqlite.New(db *sql.DB) *Driver`. You blank-import the actual SQLite library (`modernc.org/sqlite` for pure-Go, `mattn/go-sqlite3` for CGo); the driver just wants a `*sql.DB`. Also works with libSQL. SQLite is a great fit for single-node services, embedded/CLI/desktop tools, and local dev — no server to run, the queue lives in one file. There are exactly two setup details to get right, then it just works:

### SQLite setup — the two things to set

1. **`db.SetMaxOpenConns(1)`.** SQLite serializes writes (one writer at a time). Pinning the pool to a single connection serializes River's operations cleanly and avoids `SQLITE_BUSY`. Pair it with WAL mode and a `_busy_timeout` in the DSN for good measure. This is the one-liner that makes SQLite + River smooth.
2. **`PollOnly: true`.** SQLite has no LISTEN/NOTIFY, so River polls. Pickup latency is bounded by `FetchPollInterval` (default 1s) — drop it (e.g. 250ms) if you want snappier pickup.

Good to know (not problems, just characteristics):

- **Transaction type is `*sql.Tx`** — `InsertTx(ctx, tx, …)` takes a `*sql.Tx` from `db.BeginTx(ctx, nil)`, and you use `river.ClientFromContext[*sql.Tx](ctx)` inside workers.
- **`InsertMany` runs row-by-row** (a sqlc detail) rather than one batched statement; fine for typical volumes, and an upstream change is improving it via `json_each`. `InsertManyFast` (Postgres COPY) doesn't apply.
- **Single-writer by nature** — throughput is bounded by that one connection. For most single-node apps that's plenty; reach for pgx when you genuinely need many concurrent worker processes hammering the queue.

The DSN above (`?_journal_mode=WAL&_busy_timeout=5000`) plus `SetMaxOpenConns(1)` is the known-good baseline.

## riverdatabasesql (Postgres via database/sql)

```go
import (
    "database/sql"
    _ "github.com/jackc/pgx/v5/stdlib" // or lib/pq
    "github.com/riverqueue/river/riverdriver/riverdatabasesql"
)

db, _ := sql.Open("pgx", os.Getenv("DATABASE_URL"))
client, _ := river.NewClient(riverdatabasesql.New(db), &river.Config{
    PollOnly: true, // no LISTEN/NOTIFY through database/sql
    Workers:  workers,
    Queues:   map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
})
```

`riverdatabasesql.New(db *sql.DB) *Driver`. It's still Postgres under the hood — choose it only when you're locked into `database/sql` (e.g. sharing a handle with Bun or GORM) and can't introduce a `pgxpool.Pool`. No LISTEN/NOTIFY, so `PollOnly`. If you have the choice, `riverpgxv5` is faster and lower-latency.

## Migrations per driver

Both pgx and SQLite carry the **same migration versions** under a single `"main"` line; the emitted SQL differs per driver but you don't manage that — just run the migrator with the matching driver:

```go
migrator, _ := rivermigrate.New(riverpgxv5.New(pool), nil)  // or riversqlite.New(db)
_, _ = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
```

For SQLite, run migrations against the same single-connection `*sql.DB`. If your app already uses a migration tool (Goose, golang-migrate, Atlas) for its own schema, it's fine to keep River's migrations separate and run them inline at startup — that's a common, clean split (River owns `river_*` tables; your tool owns the rest).
