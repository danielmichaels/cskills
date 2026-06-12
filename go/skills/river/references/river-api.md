# River API Reference (v0.39.0)

Accurate as of River v0.39.0. Source of truth: `~/.cache/checkouts/github.com/riverqueue/river`. Signatures below are quoted/condensed from that tree — verify there if anything looks off.

## Table of contents

- [Modules & imports](#modules--imports)
- [Defining jobs: JobArgs](#defining-jobs-jobargs)
- [Defining workers: Worker[T]](#defining-workers-workert)
- [Registering workers](#registering-workers)
- [Client & Config](#client--config)
- [Inserting jobs](#inserting-jobs)
- [InsertOpts & UniqueOpts](#insertopts--uniqueopts)
- [Periodic jobs](#periodic-jobs)
- [Lifecycle: Start / Stop](#lifecycle-start--stop)
- [Subscriptions / events](#subscriptions--events)
- [Migrations: rivermigrate](#migrations-rivermigrate)
- [Testing: rivertest](#testing-rivertest)

## Modules & imports

River is a multi-module repo. Add only what you use:

```go
import (
    "github.com/riverqueue/river"
    "github.com/riverqueue/river/rivertype"                 // JobArgs, Hook, Middleware, JobRow, JobState
    "github.com/riverqueue/river/riverdriver/riverpgxv5"     // Postgres / pgx driver
    // or "github.com/riverqueue/river/riverdriver/riversqlite"
    // or "github.com/riverqueue/river/riverdriver/riverdatabasesql"
    "github.com/riverqueue/river/rivermigrate"               // schema migrations
    "github.com/riverqueue/river/rivertest"                  // test assertions / NewWorker
)
```

Each driver and `rivermigrate`/`rivertest` is a **separate Go module** with its own version — pin them to the same version as `river` itself.

## Defining jobs: JobArgs

A job type is a plain struct implementing `Kind() string`. The struct is JSON-serialized to the DB.

```go
type SendEmailArgs struct {
    To      string `json:"to"`
    Subject string `json:"subject"`
}

func (SendEmailArgs) Kind() string { return "send_email" }
```

`rivertype.JobArgs` (also re-exported as `river.JobArgs`):

```go
type JobArgs interface {
    Kind() string
}
```

Optional interfaces a JobArgs type may also implement:

```go
// Default insertion options for every job of this kind.
type JobArgsWithInsertOpts interface {
    InsertOpts() river.InsertOpts
}

// Alternate kind strings to match — use to rename Kind() safely.
type JobArgsWithKindAliases interface {
    KindAliases() []string
}

// Per-job-type hooks (insert/work lifecycle).
type JobArgsWithHooks interface {
    Hooks() []rivertype.Hook
}
```

Example with per-kind insert opts:

```go
func (SendEmailArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{
        Queue:      "email",
        MaxAttempts: 3,
    }
}
```

The job as seen inside a worker is `river.Job[T]`, which embeds the row metadata:

```go
type Job[T JobArgs] struct {
    *rivertype.JobRow      // ID, Attempt, MaxAttempts, Metadata, ScheduledAt, etc.
    Args T
}
```

## Defining workers: Worker[T]

Embed `river.WorkerDefaults[T]` and implement `Work`. The defaults supply no-op `Middleware`, `NextRetry`, and `Timeout` so you only override what you need.

```go
type SendEmailWorker struct {
    river.WorkerDefaults[SendEmailArgs]
    Mailer mailer.Mailer
    Logger *slog.Logger
}

func (w *SendEmailWorker) Work(ctx context.Context, job *river.Job[SendEmailArgs]) error {
    return w.Mailer.Send(ctx, job.Args.To, job.Args.Subject)
}
```

The full `Worker[T]` interface (override any of these beyond `Work`):

```go
type Worker[T JobArgs] interface {
    Work(ctx context.Context, job *Job[T]) error
    Timeout(job *Job[T]) time.Duration              // 0 → use Config.JobTimeout (default 1m)
    NextRetry(job *Job[T]) time.Time                // zero → use Config.RetryPolicy
    Middleware(job *rivertype.JobRow) []rivertype.WorkerMiddleware
}
```

Override `Timeout` for long jobs:

```go
func (w *EnumerateWorker) Timeout(*river.Job[EnumerateArgs]) time.Duration {
    return 5 * time.Minute
}
```

### Controlling job outcome from Work

- Return `nil` → job **completed**.
- Return any error → job **errors**; River retries per the retry policy until `MaxAttempts`, then **discarded**.
- `return river.JobSnooze(d)` → reschedule `d` from now without consuming an attempt (use when "not ready yet").
- `return river.JobCancel(err)` → cancel immediately, no more retries regardless of attempts.

## Registering workers

```go
workers := river.NewWorkers()
river.AddWorker(workers, &SendEmailWorker{Mailer: m, Logger: log})
river.AddWorker(workers, &ScanWorker{Store: s})
// AddWorker panics on duplicate kind / bad config; AddWorkerSafely returns an error instead.
```

The `*river.Workers` is then passed as `Config.Workers`. A client with no `Workers` (or `nil`) is **insert-only** — it can enqueue but runs nothing.

## Client & Config

```go
func NewClient[TTx any](driver riverdriver.Driver[TTx], config *Config) (*Client[TTx], error)
```

`river.Config` — the fields you'll actually touch (full list in source `client.go`):

```go
type Config struct {
    Queues       map[string]river.QueueConfig // omit/nil for insert-only clients
    Workers      *river.Workers               // omit/nil for insert-only clients
    MaxAttempts  int                          // default 25; per-job InsertOpts can override
    JobTimeout   time.Duration                // default 1m; -1 = no timeout
    RetryPolicy  river.ClientRetryPolicy      // default DefaultRetryPolicy (exponential backoff)
    PeriodicJobs []*river.PeriodicJob
    Middleware   []rivertype.Middleware       // wraps every Work + insert
    Hooks        []rivertype.Hook             // insert/work lifecycle hooks
    ErrorHandler river.ErrorHandler           // called on Work error / panic
    Logger       *slog.Logger                 // default slog at Info
    PollOnly     bool                         // disable LISTEN/NOTIFY; poll only (required for SQLite/database-sql)
    FetchCooldown     time.Duration           // default 100ms; min time between fetches after a fetch
    FetchPollInterval time.Duration           // default 1s; poll cadence when idle / PollOnly
    Schema       string                       // non-default Postgres schema
    ID           string                       // client id; auto-generated if empty

    // Retention (all default sensible; -1 disables deletion):
    CompletedJobRetentionPeriod time.Duration // default 24h
    CancelledJobRetentionPeriod time.Duration // default 24h
    DiscardedJobRetentionPeriod time.Duration // default 7d

    Test river.TestConfig                     // for tests (e.g. Time mocking)
}
```

```go
type QueueConfig struct {
    MaxWorkers        int           // 1..10_000; concurrency for this queue
    FetchCooldown     time.Duration // optional per-queue override
    FetchPollInterval time.Duration // optional per-queue override
}
```

Useful constants: `river.QueueDefault == "default"`, `river.PriorityDefault == 1`.

Minimal worker client:

```go
client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
    Workers: workers,
})
```

## Inserting jobs

```go
// Non-transactional:
func (c *Client[TTx]) Insert(ctx, args JobArgs, opts *InsertOpts) (*rivertype.JobInsertResult, error)
func (c *Client[TTx]) InsertMany(ctx, params []InsertManyParams) ([]*rivertype.JobInsertResult, error)

// Transactional — enqueue atomically with your business write (PREFERRED when a tx exists):
func (c *Client[TTx]) InsertTx(ctx, tx TTx, args JobArgs, opts *InsertOpts) (*rivertype.JobInsertResult, error)
func (c *Client[TTx]) InsertManyTx(ctx, tx TTx, params []InsertManyParams) ([]*rivertype.JobInsertResult, error)

// Fast bulk (Postgres COPY FROM) — faster, but NO unique-conflict handling, returns only a count:
func (c *Client[TTx]) InsertManyFast(ctx, params []InsertManyParams) (int, error)
func (c *Client[TTx]) InsertManyFastTx(ctx, tx TTx, params []InsertManyParams) (int, error)
```

```go
type InsertManyParams struct {
    Args       river.JobArgs
    InsertOpts *river.InsertOpts // may be nil
}
```

`opts` may be `nil` to use the type's `InsertOpts()` (if any) and client defaults.

### Inserting from inside a worker

Workers don't hold a client field — retrieve it from context (River injects it):

```go
func (w *ScanWorker) Work(ctx context.Context, job *river.Job[ScanArgs]) error {
    rc := river.ClientFromContext[pgx.Tx](ctx)
    _, err := rc.Insert(ctx, AssessArgs{ID: job.Args.ID}, nil)
    return err
}
```

The type parameter must match the client's `TTx` (`pgx.Tx`, `*sql.Tx`, …).

## InsertOpts & UniqueOpts

```go
type InsertOpts struct {
    Queue       string           // default "default"
    Priority    int              // 1 (highest) .. 4 (lowest); default 1
    MaxAttempts int              // overrides client MaxAttempts for this job
    ScheduledAt time.Time        // run no earlier than this (future-schedule)
    Tags        []string         // free-form labels (queryable)
    Metadata    []byte           // arbitrary JSON stored on the job
    Pending     bool             // insert in "pending" state (won't run until made available)
    UniqueOpts  UniqueOpts
}
```

```go
type UniqueOpts struct {
    ByArgs      bool                   // dedupe on (a subset of) the args
    ByPeriod    time.Duration          // dedupe within a rolling window (>= 1s)
    ByQueue     bool                   // include queue in the dedupe key
    ByState     []rivertype.JobState   // which states count as "already exists"
    ExcludeKind bool                   // omit kind from the key (rare)
}
```

**Unique semantics (v3, current):** uniqueness is enforced by a **database unique index** on a hash of `kind + selected unique properties` — fast and race-free, not advisory locks. Notes:

- If you set a custom `ByState`, it **must include the 4 required states**: `available`, `pending`, `running`, `scheduled`. Omitting one falls back to legacy advisory-lock behavior and is a common source of "the unique job inserted anyway" confusion.
- With `ByArgs`, you can dedupe on a **subset** of fields by tagging them:

```go
type RefreshArgs struct {
    TenantID int32 `json:"tenant_id" river:"unique"` // only this field enters the unique key
    Reason   string `json:"reason"`                  // ignored for uniqueness
}
func (RefreshArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}
```

A duplicate insert returns the existing job (no error); check `result.UniqueSkippedAsDuplicate`.

## Periodic jobs

```go
func river.NewPeriodicJob(
    schedule    river.PeriodicSchedule,           // e.g. river.PeriodicInterval(5*time.Minute)
    constructor func() (river.JobArgs, *river.InsertOpts),
    opts        *river.PeriodicJobOpts,
) *river.PeriodicJob

type PeriodicJobOpts struct {
    ID         string // unique across periodic jobs; auto if empty
    RunOnStart bool   // also enqueue immediately when the scheduler starts
}
```

```go
cfg.PeriodicJobs = []*river.PeriodicJob{
    river.NewPeriodicJob(
        river.PeriodicInterval(5*time.Minute),
        func() (river.JobArgs, *river.InsertOpts) { return RefreshStatsArgs{}, nil },
        &river.PeriodicJobOpts{RunOnStart: true},
    ),
}
```

Schedule helpers: `river.PeriodicInterval(d)`, `river.NeverSchedule()`. For cron expressions, implement the `PeriodicSchedule` interface (`Next(time.Time) time.Time`) — e.g. wrap `robfig/cron`.

Dynamic management at runtime: `client.PeriodicJobs().Add(...)`, `.Remove(handle)`, `.RemoveByID(id)`, `.Clear()`.

Remember: leader-only, in-memory, lost on restart (principle 5 in SKILL.md).

## Lifecycle: Start / Stop

```go
client.Start(ctx)         // begins fetching + working; non-blocking. No-op fields → insert-only, don't Start.
client.Stop(ctx)          // graceful: stop fetching, let running jobs finish (soft stop)
client.StopAndCancel(ctx) // hard: cancel running jobs' contexts immediately
<-client.Stopped()        // channel closed once fully stopped
```

Prefer `Stop` for graceful shutdown; reserve `StopAndCancel` for "shut down now". A client built with a `nil` pgx pool can still `InsertTx` (handy for rollback-only test transactions) but cannot `Start` or `Insert`.

## Subscriptions / events

```go
sub, cancel := client.Subscribe(river.EventKindJobCompleted, river.EventKindJobFailed)
defer cancel()
for event := range sub {
    // event.Kind, event.Job (*rivertype.JobRow)
}
```

`SubscribeConfig{ChanSize, Kinds}` lets you size the buffer (default 1000; **events are dropped if the channel overflows** — keep the consumer fast). Common kinds: `EventKindJobCompleted`, `EventKindJobFailed`, `EventKindJobCancelled`, `EventKindJobSnoozed`.

## Migrations: rivermigrate

River owns its schema (the `river_job` table etc.) and ships migrations. Run them programmatically at startup or via the CLI.

```go
migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil) // nil → default Config (line "main")
res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
for _, v := range res.Versions {
    log.Info("river migration", "direction", res.Direction, "version", v.Version)
}
```

```go
func rivermigrate.New[TTx any](driver riverdriver.Driver[TTx], config *Config) (*Migrator[TTx], error)
type Config struct { Line string; Logger *slog.Logger; Schema string } // Line defaults to "main"

func (m *Migrator[TTx]) Migrate(ctx, direction Direction, opts *MigrateOpts) (*MigrateResult, error)
type MigrateOpts struct { DryRun bool; MaxSteps int; TargetVersion int }
// DirectionUp / DirectionDown
```

`Migrate` (not the deprecated `MigrateTx`) is preferred — some migrations can't run inside a transaction. Both pgx and SQLite drivers carry the same migration versions under a single `"main"` line; the SQL differs per driver but you don't manage that.

CLI equivalent (`go run github.com/riverqueue/river/cmd/river`):

```
river migrate-up   --database-url "$DATABASE_URL"
river migrate-down --database-url "$DATABASE_URL" --max-steps 1
river migrate-get  --version 6 --up --database-url "sqlite://"   # emit SQL (sqlite:// hints SQLite SQL)
river validate     --database-url "$DATABASE_URL"
```

## Testing: rivertest

```go
import "github.com/riverqueue/river/rivertest"
```

**Assert a job was inserted** (exactly one of the kind; fails otherwise):

```go
job := rivertest.RequireInserted(ctx, t, riverpgxv5.New(pool), SendEmailArgs{To: "a@b.com"}, nil)
// or RequireInsertedTx(ctx, t, tx, args, opts) inside a test transaction
rivertest.RequireNotInserted(ctx, t, driver, SomeArgs{}, nil)
rivertest.RequireManyInserted(ctx, t, driver, []rivertest.ExpectedJob{{Args: A{}}, {Args: B{}}}) // exact ordered list
```

```go
type RequireInsertedOpts struct {
    Queue string; Priority int; MaxAttempts int
    ScheduledAt time.Time; Tags []string; State rivertype.JobState; Schema string
}
```

**Drive a worker end-to-end in a test** (inserts + works a job in a tx, rolled back after):

```go
w := rivertest.NewWorker(t, driver, &river.Config{}, &SendEmailWorker{Mailer: fake})
res, err := w.Work(ctx, t, tx, SendEmailArgs{To: "a@b.com"}, nil) // res.WorkResult
```

`rivertest.WorkContext(ctx, client)` injects a client so `river.ClientFromContext` works inside a worker under test.

> Note: a real heavy-usage codebase chose **not** to use `rivertest` at all — see `references/patterns.md` for the lighter "call `Work` directly + interface seam" approach. Both are valid; pick by how much River machinery the test actually needs to exercise.
