# River Anti-Patterns

Each entry is a mistake that passes local testing and bites in production, with the fix. Use this list in review/debug mode.

## 1. Enqueueing outside the transaction the job depends on

```go
// ✗ WRONG — two separate operations, no atomicity
scanID, _ := store.CreateScan(ctx, params)         // commits immediately
client.Insert(ctx, ScanArgs{ScanID: scanID}, nil)  // separate; may run before/without the row
```

If the process dies between the two lines, you've created a row with no job (work never happens) or — with the order reversed — a job with no row (worker reads nothing). Even without a crash, a worker can pick the job up before the row is visible.

```go
// ✓ RIGHT — one transaction
tx, _ := pool.Begin(ctx); defer tx.Rollback(ctx)
scanID, _ := store.WithTx(tx).CreateScan(ctx, params)
client.InsertTx(ctx, tx, ScanArgs{ScanID: scanID}, nil)
tx.Commit(ctx)
```

This is the single most important River discipline. If you review one thing, review this.

## 2. Fat args — passing whole objects, or state that goes stale

```go
// ✗ WRONG
type ProcessArgs struct {
    User    *User           // huge, stale by the time it runs, may contain secrets
    DBConn  *pgxpool.Pool   // not serializable — will break
}
```

Args are JSON in a DB column that may sit for minutes/hours/across deploys. Pass the minimal IDs and re-fetch inside `Work`:

```go
// ✓ RIGHT
type ProcessArgs struct { UserID int64 `json:"user_id"` }
func (w *Worker) Work(ctx context.Context, job *river.Job[ProcessArgs]) error {
    user, err := w.Store.GetUser(ctx, job.Args.UserID) // current state
    // ...
}
```

## 3. Renaming `Kind()` without an alias

```go
// Was: func (FooArgs) Kind() string { return "foo" }
func (FooArgs) Kind() string { return "process_foo" } // ✗ orphans every queued "foo" job
```

Jobs already in the table still carry kind `"foo"`; no worker matches → they error to discarded. Keep the old name reachable while they drain:

```go
func (FooArgs) Kind() string          { return "process_foo" }
func (FooArgs) KindAliases() []string { return []string{"foo"} } // ✓
```

## 4. Non-idempotent `Work`

```go
// ✗ WRONG — at-least-once means this can double-charge
func (w *Worker) Work(ctx context.Context, job *river.Job[ChargeArgs]) error {
    return w.payments.Charge(ctx, job.Args.CustomerID, job.Args.Cents)
}
```

A crash after the charge but before the ack reruns the job. Guard it:

```go
// ✓ RIGHT — idempotency key / check-then-act / DB unique constraint
func (w *Worker) Work(ctx context.Context, job *river.Job[ChargeArgs]) error {
    return w.payments.Charge(ctx, job.Args.CustomerID, job.Args.Cents,
        payments.IdempotencyKey(fmt.Sprintf("job-%d", job.ID)))
}
```

`job.ID` is stable across retries of the same job — it makes a good idempotency key.

## 5. Long job, default timeout

```go
// ✗ A job that scans the network for minutes...
func (w *EnumerateWorker) Work(ctx context.Context, job *river.Job[EnumArgs]) error {
    return w.crawl(ctx) // takes 4 minutes; killed at 1m by default JobTimeout
}
```

```go
// ✓ Override Timeout for the worker
func (w *EnumerateWorker) Timeout(*river.Job[EnumArgs]) time.Duration { return 10 * time.Minute }
```

## 6. SQLite client missing its two setup lines

SQLite + River works great with two small settings in place. Forget them and you can see `SQLITE_BUSY` under concurrency — not a SQLite problem, just a missing config:

```go
// ✗ Missing the setup — single-connection pin and PollOnly not set
db, _ := sql.Open("sqlite", "app.db")
client, _ := river.NewClient(riversqlite.New(db), &river.Config{Workers: w, Queues: q})
```

```go
// ✓ The known-good baseline
db, _ := sql.Open("sqlite", "file:app.db?_journal_mode=WAL&_busy_timeout=5000")
db.SetMaxOpenConns(1)        // serialize writes — SQLite is single-writer
client, _ := river.NewClient(riversqlite.New(db), &river.Config{
    Workers: w, Queues: q, PollOnly: true, // no LISTEN/NOTIFY on SQLite
})
```

## 7. Holding a transaction across slow external work

```go
// ✗ WRONG — row locks held across a multi-minute HTTP/DNS sweep → contention/deadlocks
tx, _ := pool.Begin(ctx)
results := w.callSlowExternalThing(ctx) // minutes
store.WithTx(tx).SaveAll(ctx, results)
tx.Commit(ctx)
```

Do the slow work first with no tx open, then persist in short transactions (see patterns.md §6).

## 8. Relying on periodic jobs for durability or exactly-once

Periodic schedules are in-memory and leader-only. After a restart or leader change the in-memory schedule resets; a missed tick is just missed. Don't build "charge every customer on the 1st" purely on a periodic job — back it with a durable ledger/cursor the job reconciles against, and use `RunOnStart: true` to recover quickly after a restart.

## 9. Unbounded `MaxWorkers` against a rate-limited dependency

```go
// ✗ 500 workers all hammering a 3rd-party API that allows 10 req/s
Queues: map[string]river.QueueConfig{ "external": {MaxWorkers: 500} }
```

`MaxWorkers` is your concurrency knob. Size it to what the downstream can take; put slow/rate-limited jobs on their own queue so they don't consume the workers your fast jobs need.

## 10. One client doing everything in a multi-process deploy

Running workers inside every process (including the HTTP API) means every deploy/restart of the API also churns workers, and you can't scale them separately. Split into insert-only clients (API) and worker clients (dedicated process) — same `NewClient`, different `Config` (patterns.md §2).

## 11. Custom `ByState` missing a required state

```go
// ✗ Expecting dedupe, but omitting required states silently changes semantics
UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateRunning}}
```

A custom `ByState` must include all four of `available`, `pending`, `running`, `scheduled`, or River falls back to legacy advisory-lock behavior and the dedupe won't work as you expect. If you just want "don't enqueue a duplicate that hasn't finished", omit `ByState` and take the default.

## 12. Forgetting `Start()`, or calling it on an insert-only client

- Jobs inserted but nothing runs? The worker process may never have called `client.Start(ctx)`, or the queue isn't in the `Queues` map, or no worker is registered for the kind.
- Conversely, don't `Start()` an insert-only client (one with nil `Workers`/`Queues`) — it has nothing to do.
