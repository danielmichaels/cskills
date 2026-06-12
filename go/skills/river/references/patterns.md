# River Production Patterns

Patterns distilled from a codebase that uses River heavily across a multi-process deployment (an HTTP API plus dedicated worker processes, with a scan→assess job pipeline). These are conventions that earn their keep at scale — adopt the ones that fit; they're not mandatory.

## 1. One cohesive `jobs` package

Keep everything River in a single package (e.g. `internal/jobs/`): a `New(ctx, cfg) (*river.Client[pgx.Tx], error)` factory that builds the client, registers workers, runs migrations, and declares periodic jobs; plus one file per job family (args + worker + registration together). The rest of the app depends on the `*river.Client` and a small set of enqueue functions — not on River internals.

```
internal/jobs/
├── river.go            // Config, New() factory, migrations, worker registry, periodic jobs
├── scan_jobs.go        // ScanCertificateArgs + ScanCertificateWorker + ...
├── assess_jobs.go      // downstream fan-out workers
├── send_email_job.go
├── middleware.go       // timing/correlation middleware
└── *_test.go
```

## 2. Insert-only mode vs worker mode (the key scaling split)

The same `river.NewClient` builds two very different clients depending on a flag. Your **HTTP server** enqueues jobs but runs no workers; a **separate worker process** runs the full registry. This lets you scale request handling and job execution independently, and deploy/restart workers without dropping requests.

```go
type Config struct {
    PgxPool     *pgxpool.Pool
    WorkerCount int
    AddWorkers  bool // false = insert-only (the API server); true = full workers
    // ...deps injected into workers: Store, Mailer, Resolver, Logger
}

func New(ctx context.Context, cfg Config) (*river.Client[pgx.Tx], error) {
    // Always run migrations first.
    migrator, err := rivermigrate.New(riverpgxv5.New(cfg.PgxPool), nil)
    if err != nil { return nil, err }
    if _, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
        return nil, err
    }

    riverConfig := &river.Config{}
    if cfg.AddWorkers {
        rw := river.NewWorkers()
        river.AddWorker(rw, &ScanCertificateWorker{Store: cfg.Store, Logger: cfg.Logger})
        // ...all workers...
        riverConfig.Workers = rw
        riverConfig.Queues = map[string]river.QueueConfig{
            river.QueueDefault: {MaxWorkers: cfg.WorkerCount},
            queueScanner:       {MaxWorkers: cfg.WorkerCount},
        }
        riverConfig.PeriodicJobs = periodicJobs(cfg)
        riverConfig.Middleware = []rivertype.Middleware{&CorrelationMiddleware{}, &TimingMiddleware{Logger: cfg.Logger}}
        riverConfig.Hooks = []rivertype.Hook{&CorrelationInsertHook{}}
    }
    return river.NewClient(riverpgxv5.New(cfg.PgxPool), riverConfig)
}
```

The insert-only client leaves `Workers`/`Queues`/`PeriodicJobs` nil and **never calls `Start()`**. Only the worker process calls `client.Start(ctx)`.

## 3. A shared identity struct embedded into args

When a family of jobs all operate on the same entity, define one identity struct and embed it by value. Keeps the args minimal, consistent, and refactor-safe.

```go
type DomainJobArgs struct {
    DomainUID  string `json:"domain_uid"`
    DomainName string `json:"domain_name"`
    ScanID     int64  `json:"scan_id"`
    TenantID   int32  `json:"tenant_id"`
}

type ScanCertificateArgs struct{ DomainJobArgs }
func (ScanCertificateArgs) Kind() string            { return "scan_certificate" }
func (ScanCertificateArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{Queue: queueScanner} }
```

Note `InsertOpts()` as a method on the args type — every insert of that kind picks up the queue automatically, so call sites pass `nil` opts.

## 4. Atomic fan-out: InsertManyTx with the business write

The entry point that creates a unit of work enqueues all its jobs in the **same transaction** as the row it writes. The caller owns the tx, so creation and enqueue are atomic (principle 1).

```go
func EnqueueDomainScan(ctx context.Context, rc *river.Client[pgx.Tx], tx pgx.Tx, ident DomainJobArgs) error {
    params := []river.InsertManyParams{
        {Args: ResolveDomainArgs{DomainJobArgs: ident}},
        {Args: ScanCertificateArgs{DomainJobArgs: ident}},
        {Args: ScanDNSSECArgs{DomainJobArgs: ident}},
    }
    if _, err := rc.InsertManyTx(ctx, tx, params); err != nil {
        return fmt.Errorf("enqueue scan jobs: %w", err)
    }
    return nil
}
```

## 5. Job chaining from inside a worker

A worker that finishes one phase enqueues the next. Get the client from context; open a fresh tx if you want the follow-on enqueue to be atomic with this worker's own DB writes (the original enqueueing tx is long committed by now).

```go
func (w *ScanCertificateWorker) Work(ctx context.Context, job *river.Job[ScanCertificateArgs]) error {
    // ...do the scan, write results...
    rc := river.ClientFromContext[pgx.Tx](ctx)

    tx, err := w.PgxPool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil { return err }
    defer tx.Rollback(ctx)

    if err := w.Store.WithTx(tx).SaveResult(ctx, result); err != nil { return err }
    if _, err := rc.InsertTx(ctx, tx, AssessCertificateArgs{DomainJobArgs: job.Args.DomainJobArgs}, nil); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
```

If there's no DB write to tie it to, a plain `rc.Insert(ctx, args, nil)` is acceptable — but prefer `InsertTx` whenever a tx is in hand so a crash can't leave the chain half-enqueued.

## 6. Don't hold a transaction across slow work

If a worker does multi-minute network/IO, do **not** keep a row-locking tx open across it — you'll cause lock contention and deadlocks. Drain the slow work first (no tx), then persist each result in its own short transaction.

```go
hosts := w.discoverHostsOverNetwork(ctx)        // minutes; no tx held
for _, h := range hosts {
    tx, _ := w.PgxPool.BeginTx(ctx, pgx.TxOptions{})
    _ = w.Store.WithTx(tx).Upsert(ctx, h)
    _ = tx.Commit(ctx)
}
```

Pair this with a per-worker `Timeout()` override so River doesn't kill the job at the default 1 minute.

## 7. Trace correlation via Hook + Middleware

To propagate a trace/correlation ID from the request that enqueued a job through to the worker (and onward to child jobs), use the complementary pair:

- An **insert hook** (`InsertBegin`) stamps the current context's trace ID into the job's `Metadata` JSON at insert time.
- A **work middleware** reads it back out of `Metadata` and restores it into the worker's context before `Work` runs.

Because child jobs enqueued inside a worker inherit that context, the whole pipeline shares one trace ID. This is the cleanest way to get end-to-end observability across async hops. Sketch:

```go
type CorrelationInsertHook struct{}
func (CorrelationInsertHook) InsertBegin(ctx context.Context, params *rivertype.JobInsertParams) error {
    if id := tracing.FromContext(ctx); id != "" {
        params.Metadata = mergeJSON(params.Metadata, map[string]string{"trace_id": id})
    }
    return nil
}

type CorrelationMiddleware struct{}
func (CorrelationMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
    if id := traceIDFromMetadata(job.Metadata); id != "" {
        ctx = tracing.WithID(ctx, id)
    }
    return doInner(ctx)
}
```

A `TimingMiddleware` alongside it that logs `kind/queue/attempt/duration/outcome` (DEBUG on success, ERROR on failure) gives you cheap per-job telemetry without touching every worker.

## 8. Unique jobs to coalesce redundant work

When the same logical job being enqueued twice in quick succession is wasteful (e.g. "refresh stats for tenant X" fired from many concurrent writes), make it unique by args so River collapses them into one pending job:

```go
func (RefreshTenantStatsArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}
```

## 9. Testability via an interface seam (no River in business tests)

Rather than spin up River in service/handler tests, define a narrow scheduler interface that the service depends on, implement it with a thin River adapter in production and a fake in tests. River stays entirely out of business-logic tests.

```go
// Production code depends on this, not on *river.Client:
type DomainScanScheduler interface {
    Schedule(ctx context.Context, tx pgx.Tx, ident DomainJobArgs) error
}

// Adapter (prod):
type riverScheduler struct{ rc *river.Client[pgx.Tx] }
func (s riverScheduler) Schedule(ctx context.Context, tx pgx.Tx, ident DomainJobArgs) error {
    return EnqueueDomainScan(ctx, s.rc, tx, ident)
}

// Fake (test):
type fakeScheduler struct{ calls int }
func (f *fakeScheduler) Schedule(context.Context, pgx.Tx, DomainJobArgs) error { f.calls++; return nil }
```

For **worker** tests, call `Work` directly with a hand-built job — no client needed:

```go
w := &PurgeDNSCacheWorker{Store: queries, Logger: testLogger}
err := w.Work(ctx, &river.Job[PurgeDNSCacheArgs]{Args: PurgeDNSCacheArgs{}})
```

(`rivertest.NewWorker` is the heavier alternative when you want River to exercise insert + unique handling + middleware around the work — see `references/river-api.md`.)

## 10. Graceful shutdown

Use `Stop` (soft — let in-flight jobs finish), not `StopAndCancel`, for normal shutdown:

```go
if err := client.Stop(ctx); err != nil {
    logger.Error("failed to stop River client", "error", err)
}
```

Reserve `StopAndCancel` for forced/timed-out shutdown where you accept cancelling running jobs (they'll be retried later, since they error out un-acked).
