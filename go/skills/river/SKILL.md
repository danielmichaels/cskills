---
name: river
description: Comprehensive reference and decision-helper for the River background job queue (github.com/riverqueue/river) in Go. Use this skill aggressively whenever the user works with River — defining jobs/workers, configuring river.Client, enqueueing jobs (Insert/InsertTx/InsertMany), periodic/cron jobs, unique jobs, retries, rivermigrate, rivertest, or the riverpgxv5/riversqlite/riverdatabasesql drivers — even if they don't say "River" explicitly. Trigger on phrases like "background job", "job queue", "worker pool in Go", "enqueue a job", "JobArgs", "Kind()", "river.Worker", "InsertTx", "periodic job", "cron job in Go", "process jobs", "queue config", or any import under github.com/riverqueue/river. Also use when reviewing or debugging River code (jobs not running, stuck jobs, unique conflicts, retries, transactional enqueue) or when choosing between the Postgres (pgx) and SQLite drivers.
---

# River — Go Background Jobs

River is a transactional background job queue for Go backed by a SQL database (Postgres or SQLite). This skill teaches its API, the trade-offs between its drivers, and the conventions a real heavy-usage codebase converged on. It also acts as a thinking partner: at design time it asks hard questions, at review time it flags violations, at debug time it maps symptoms to causes.

Pinned to **River v0.39.0** (June 2026). If you're unsure an API still matches, check the cached source at `~/.cache/checkouts/github.com/riverqueue/river` (run the `librarian` skill to refresh it) before asserting.

## When this skill applies

Use it whenever work touches River:

- Adding a new job type (args + worker) or a new queue
- Setting up or changing the `river.Client` / `river.Config`
- Enqueueing jobs, especially alongside a database write
- Periodic/cron jobs, unique jobs, retries, custom timeouts
- Migrations (`rivermigrate`) and driver choice (pgx vs SQLite)
- Testing workers, or reviewing/debugging existing job code

## The one thing to internalize first

**River lives in your database. A job insert is a row insert.** That single fact drives everything else: you can enqueue a job *in the same transaction* as the business write it depends on, so either both commit or neither does. This is River's reason to exist over an in-memory or Redis queue. If you take nothing else from this skill, take this: **when a job depends on a row you're writing, insert the job in that same transaction.**

## Core principles (the non-negotiables)

### 1. Enqueue in the same transaction as the write the job depends on

If a job processes a row, the job insert and the row write must be atomic. Use `InsertTx` / `InsertManyTx` with the same `tx`:

```go
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)
scanID, _ := store.WithTx(tx).CreateScan(ctx, params)   // business write
_, _ = client.InsertManyTx(ctx, tx, []river.InsertManyParams{ // enqueue in same tx
    {Args: ScanCertificateArgs{ScanID: scanID}},
})
tx.Commit(ctx) // both land together, or neither does
```

**Why it matters.** Enqueue *outside* the transaction and you get one of two classic bugs: the tx rolls back but the job is already queued (worker processes a row that never existed), or the job is picked up before the row commits (worker reads stale/missing data). Both are race conditions that only show under load. Transactional enqueue makes them impossible.

### 2. Job args are a serialization contract, not a function call

Args are JSON-marshalled into a DB column and may sit there across deploys, restarts, and retries. Treat them like a wire format:

- **Pass IDs, not objects.** Store `DomainID int32`, not a hydrated `*Domain`. Re-fetch current state inside `Work` — the row may have changed since enqueue.
- **Keep them small and flat.** No secrets, no large blobs, no `context`, no DB handles.
- **Stay backward-compatible.** A worker may dequeue an arg shape an older deploy wrote. Add fields, don't repurpose them.

### 3. `Kind()` is a permanent identifier

The string `Kind()` returns is how River matches a queued job to its worker. Rename it and every in-flight job of the old kind becomes unworkable (no registered worker → it errors out). To rename safely, keep the old name reachable via `KindAliases() []string`.

### 4. Workers must be safe to run more than once

River is **at-least-once**. A worker can crash after a side effect but before acking, and the job will run again. Design `Work` to tolerate re-execution: upsert instead of insert, check-then-act, make external calls idempotent (idempotency keys), or guard with the DB. "It usually runs once" is not a correctness argument.

### 5. Periodic jobs are leader-only and in-memory

Periodic jobs fire only on the elected leader client, and the schedule lives in memory — it's lost on restart/re-election. They're for "roughly every N", not for durable scheduling or exactly-once. Use `RunOnStart: true` to hedge long intervals. Don't build billing on them.

## Decision tree — which driver?

River abstracts the database behind a `riverdriver.Driver[TTx]`. Pick by concurrency and deployment shape. See `references/drivers.md` for full setup of each.

```
Need real concurrency, multiple processes, or low-latency pickup?
  └─ YES → riverpgxv5 (Postgres + pgx/v5)         ← the default. LISTEN/NOTIFY, batched inserts.
  └─ NO, single-node / embedded / dev / CLI tool?
        └─ riversqlite (SQLite)                     ← simple, no server, queue-in-a-file. Two setup
                                                       lines: SetMaxOpenConns(1) + PollOnly. Then it's smooth.
Already on database/sql + an ORM (Bun/GORM) on Postgres?
  └─ riverdatabasesql (Postgres via database/sql)   ← PollOnly; use when pgxpool isn't an option.
```

`TTx` is the transaction type and it leaks into your types: pgx drivers give you `*river.Client[pgx.Tx]`, the SQL drivers give `*river.Client[*sql.Tx]`. Pick early — switching later touches every `InsertTx` call site.

## Design mode — ask before writing

Before adding a job type, get clear answers:

- **What's the trigger?** User action in a request handler (enqueue in that tx), a periodic sweep, or a fan-out from another job?
- **What does the worker need?** The minimal set of IDs to re-fetch state. Resist passing the whole object.
- **Is it idempotent?** If re-running would double-charge / double-send, how is that guarded?
- **Should duplicates collapse?** If enqueuing the same logical job twice is wasteful, use `UniqueOpts` (e.g. `{ByArgs: true}`) so River coalesces them.
- **Which queue?** Long/slow jobs (minutes) belong on their own queue with its own `MaxWorkers`, so they don't starve fast jobs. Default queue is fine for short work.
- **What's the timeout?** Default `JobTimeout` is 1 minute. Override per-worker via `Timeout()` for long jobs, or they'll be killed mid-run.

## Review mode — what to look for

When reviewing River code, scan for these (most are subtle and pass tests):

- **Enqueue outside the transaction** of the write it depends on → principle 1. The #1 River bug.
- **Fat args** — whole structs, secrets, things that go stale → principle 2.
- **A renamed `Kind()`** with no `KindAliases` → orphans in-flight jobs.
- **Non-idempotent `Work`** — a raw `INSERT`, an unguarded external charge/email → principle 4.
- **Missing per-worker `Timeout()`** on a job that does network/IO for minutes → killed at 1 min.
- **SQLite client missing `SetMaxOpenConns(1)` + `PollOnly`** — the two-line setup; add them and SQLite runs clean.
- **Periodic job relied on for durability or exactly-once** → principle 5.
- **`client.Insert` from inside a `Work` method when a tx is available** — prefer `InsertTx` to keep fan-out atomic (gecko has a TODO flagging exactly this).
- **Unbounded `MaxWorkers`** on a queue whose jobs hit a rate-limited dependency.

## Debug mode — symptom → cause

| Symptom | Likely cause |
|---|---|
| Jobs inserted but never run | No worker registered for that `Kind()`; client started with `AddWorkers: false`; queue not in `Queues` map; or no client `Start()` called |
| Jobs run twice / double side effects | Normal at-least-once — worker isn't idempotent (principle 4) |
| `SQLITE_BUSY` / "database is locked" | SQLite driver without `SetMaxOpenConns(1)`; concurrent writers |
| Job picked up slowly (~seconds) | `PollOnly`/SQLite with default `FetchPollInterval` (1s) and no LISTEN/NOTIFY — lower the interval or move to pgx |
| Unique job inserted anyway / conflict errors | `ByState` missing one of the 4 required states (available, pending, running, scheduled); or expecting old advisory-lock semantics |
| Worker errors "unknown job kind" | `Kind()` renamed without `KindAliases`; worker not added to the registry |
| Periodic job didn't fire after restart | In-memory schedule reset; not the leader; use `RunOnStart` |
| Job processes a missing/stale row | Enqueued outside the tx (principle 1) |

## River UI (the web dashboard)

River ships a web UI for browsing/managing jobs. How you run it depends on the driver, and both ways are easy:

- **Postgres** → run the standalone Docker image pointed at `DATABASE_URL`:
  `docker run -p 8080:8080 -e RIVER_HOST= -e DATABASE_URL="postgres://…" ghcr.io/riverqueue/riverui:latest`
- **SQLite** → embed the handler in your Go app (the standalone image is Postgres-only). `riverui.NewEndpoints(client, nil)` is generic over the driver, so a `riversqlite` client works; mount `riverui.NewHandler(...)` on your mux and containerize your app with the DB file as a volume.

Full setup (env vars, compose, embed snippet, Dockerfile for the SQLite case) is in `references/river-ui.md`.

## Reference files

Read the one matching your task — don't load all of them by default:

- **`references/river-api.md`** — accurate v0.39.0 API: `Config` fields, `Insert*` signatures, `Worker[T]`, `InsertOpts`/`UniqueOpts`, periodic jobs, `rivermigrate`, `rivertest`, lifecycle. Read when writing or verifying any River call.
- **`references/drivers.md`** — full setup for pgx, SQLite, and database/sql: constructors, connection config, migrations per driver, and the two SQLite setup lines. Read when wiring up a client or choosing a driver.
- **`references/patterns.md`** — production patterns from a heavy-usage codebase: insert-only vs worker mode, job chaining/fan-out, the trace-correlation hook+middleware, the shared identity arg struct, the interface seam for testability. Read when structuring a real app's job layer.
- **`references/anti-patterns.md`** — the specific mistakes that bite, with the fix for each. Read during review/debug.
- **`references/river-ui.md`** — running River UI: standalone Docker image for Postgres, embedding for SQLite, all env vars, and a containerized-embed Dockerfile. Read when setting up the dashboard.

## Style notes for using this skill

Match the surrounding codebase's conventions over anything here — these are defaults, not laws. Keep the job package cohesive: args, worker, and registration for a job type live together. Inject worker dependencies at construction (`&FooWorker{Store: ..., Logger: ...}`); don't reach for globals. And lead with the transaction question — for most jobs, "where's the tx?" is the whole design.
