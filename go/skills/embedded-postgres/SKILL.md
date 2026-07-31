---
name: embedded-postgres
description: Run a real Postgres inside the Go process for tests and local dev, so `go test ./...` and `task dev` need no installed or running database. Use this skill whenever the user works with github.com/fergusstrange/embedded-postgres, an internal/embeddedpg package, a shared test-database helper, TestMain database setup, or asks how to test Go code against real Postgres instead of mocks/sqlmock/dockertest/testcontainers. Trigger on "embedded postgres", "tests need a database", "spin up postgres for tests", "test database helper", "TruncateAll", "shared postgres in tests", "postgres orphan", "leftover temp dirs", "embeddedpg-data", "pg_ctl won't stop", "tests leave postgres running", "port already in use in tests", or when reviewing test bootstrap that mocks the DB layer. Also use when a project needs zero-credential local dev with a persistent dev database.
---

# Embedded Postgres for Go tests and local dev

Run the real database, not a mock. `github.com/fergusstrange/embedded-postgres`
downloads a Postgres binary once, extracts it, and runs it as a child process.
Tests get real SQL — real constraints, real enum ordering, real `ON CONFLICT`,
real partial indexes — with no `docker`, no CI service container, and no
developer setup step.

## When this skill applies

- A Go project whose data layer is Postgres (sqlc, pgx, sqlx, GORM).
- Test suites currently using mocks, `sqlmock`, dockertest, or testcontainers.
- Local dev that should run with zero real credentials.
- Any symptom in [Debug mode](#debug-mode--symptom--cause) — orphaned
  postgres processes, leftover temp dirs, ports already in use.

## The one thing to internalize first

**The lifecycle is the hard part, not the startup.** Starting Postgres is ten
lines. Everything that goes wrong afterwards is a process or a directory that
outlived the thing that created it. Design for the kill: `Stop()` will *not*
run under SIGKILL, `go test -timeout`, or a panicking test binary, so recovery
cannot depend on it.

## Core principles (the non-negotiables)

1. **One instance per test binary, not per test.** Starting Postgres costs
   ~1s; per-test instances make a suite unusable. Share one via a package-level
   singleton behind a mutex, and isolate tests with `TRUNCATE`, not restarts.

2. **Isolate with TRUNCATE ... RESTART IDENTITY CASCADE.** Deterministic, fast,
   and it resets sequences so id-sensitive assertions stay stable. Call it at
   the top of each test that needs a clean slate.

3. **`TestMain` owns the lifecycle, and must trap signals.** A bare
   `defer cleanup()` in `TestMain` does not run on Ctrl-C. Trap SIGINT/SIGTERM
   and clean up there too. See `references/testhelpers.go`.

4. **Never trust `pg_ctl stop`.** It can report success while the postmaster is
   still alive. Read `<dataDir>/postmaster.pid` *before* stopping (Postgres
   deletes it early in shutdown), then poll and SIGKILL. See
   `references/lifecycle.go`.

5. **Sweep orphans on startup.** Because the kill paths bypass your cleanup,
   the only reliable reclaim point is the *next* run. Sweep stale directories
   when you start. This is what makes the system self-healing.

6. **Temp dirs are owned; persistent dirs are not.** Track which directories
   you created (`ownedDataDir`, `ownedRuntimeDir`) and delete only those.
   Deleting a user-supplied dev data dir on an error path destroys their
   database.

7. **Pin the binaries directory, and put the version in the path.** Point
   `BinariesPath` at `~/.embedded-postgres-go/extracted/<version>` so every run
   reuses one extraction instead of re-downloading — but never share one
   directory across versions. The library skips extraction whenever
   `bin/pg_ctl` exists there and never checks which version it is, so an
   unversioned path silently runs whichever major extracted first. `initdb`
   then stamps *that* version into `PG_VERSION`, and the next start compares it
   against the version you asked for, decides the data directory is foreign,
   and deletes it. Two projects on different majors quietly wipe each other's
   dev database on every restart, with nothing logged.

8. **Give each instance its own `RuntimePath`.** Parallel packages otherwise
   race on extraction. But see principle 6 — a *persistent* instance should get
   a stable runtime dir beside its data dir, or every dev restart leaks one.

9. **Keep the dev data dir out of your watcher's scratch directory.** If you
   use `air` (`tmp_dir = "tmp"`), putting the database in `tmp/` means a stray
   clean deletes it. Use `./.data/postgres`.

10. **Adopt a live instance instead of failing — but only one you can prove is
    yours.** For persistent dev use, an occupied port is normal: air SIGKILLs
    the app on reload, so Postgres routinely outlives the process that started
    it, and refusing outright breaks `task dev` on every save. Return a handle
    to the running instance, and make `Stop()` on it a no-op — you did not
    start it, so you must not stop it.

    Adopting *blindly* is the trap. Read `postmaster.pid` first: line 2 is the
    data directory and line 4 is the port, so you can confirm the occupant is
    the postmaster for **this** data dir without opening a connection. Anything
    else must be a hard error naming the port.

    Two projects defaulting to the same port is all it takes. The lucky version
    is a confusing `FATAL: database "x" does not exist`. The unlucky version is
    two checkouts of one project — same slug, so same database name — where
    adoption succeeds and migrations run into the *other* checkout's database,
    reporting nothing. Derive the dev port per project so this is rare, and
    keep the check so it is loud.

## Decision tree — which testing approach?

| Situation | Use |
|---|---|
| Go + Postgres, want real SQL semantics | **embedded-postgres** |
| Need Postgres extensions (PostGIS, TimescaleDB) | testcontainers — embedded ships plain Postgres |
| Need many DB versions in a matrix | testcontainers, or embedded with `Version()` per job |
| Non-Go project | testcontainers / docker compose |
| Team already runs Docker everywhere in CI | either; embedded is still faster and simpler |

Embedded wins when you want `git clone && go test ./...` to work on a laptop
with nothing installed. That is its whole value proposition — do not trade it
away casually.

## Design mode — ask before writing

- **Shared or per-package instance?** Per test *binary* is the default. Each Go
  package is its own binary, so `go test ./...` already gives you isolation
  between packages, in parallel.
- **Which port?** Random free port for tests (parallel packages collide
  otherwise); a fixed port for dev, so `psql` and GUI clients have a stable
  target.
- **Does dev persist?** Tests: ephemeral temp dir, deleted on stop. Dev:
  persistent dir so data survives restarts.
- **What resets between tests?** Usually all application tables. Leave job-queue
  tables (River) alone if the suite asserts against them — truncating mid-suite
  races their maintenance services.

## Review mode — what to look for

- `defer` cleanup in `TestMain` with no signal handling → orphans on Ctrl-C.
- `pg.Stop()` with its error ignored *and* no PID force-kill → silent orphans.
- Cleanup that deletes a directory it did not create.
- `os.MkdirTemp` per run with no sweep → unbounded temp growth. On macOS this
  shows up as `/var/folders/**` filling with hundreds of MB of `data-*` dirs.
- Tests calling `t.Parallel()` while sharing one database without truncation
  discipline — they will flake.
- A fixed port in tests → collisions when packages run in parallel.
- A `BinariesPath` with no version segment → see principle 7. Grep for it: any
  project sharing `~/.embedded-postgres-go/extracted` with a project on a
  different major loses its dev database on every restart.
- Adoption on `portInUse` alone, with no `postmaster.pid` check → principle 10.
  The tell is a `Start` that builds a DSN and returns it without ever
  establishing what is on the other end.
- A dev port that is the same literal in every project generated from one
  template → guarantees the collision principle 10 describes.

## Debug mode — symptom → cause

| Symptom | Cause |
|---|---|
| `port already in use` on every test run | Orphaned postmaster from a killed run. Sweep, or kill by data dir. |
| Temp dir fills with `embeddedpg-data-*` | No startup sweep; `Stop` never ran. |
| More `rt-` dirs than `data-` dirs | Persistent (dev) instances leaking runtime dirs — they create a runtime dir but no data dir. Give dev a stable runtime path. |
| `Stop()` returns nil, process still alive | `pg_ctl stop` lying. Read `postmaster.pid` first, then poll + SIGKILL. |
| Data dir deleted under a running instance | A cleanup that checked liveness incorrectly. See the warning below. |
| Dev database empty after a clean | Data dir lived inside the watcher's scratch dir. |
| Dev database empty after *every* restart, nothing logged | `BinariesPath` shared across versions, so the running major does not match `PG_VERSION` and the data dir is deleted as foreign. Check what `<binariesPath>/bin/postgres --version` actually prints. |
| `FATAL: database "x" does not exist` on a dev boot that has worked before | The port was adopted from a *different* project's postmaster. `lsof -nP -iTCP:<port> -sTCP:LISTEN` then check that pid's `-D` argument. |
| Tables from another project appear in this one's database | Same cause, but the database names matched, so adoption succeeded silently. See principle 10. |
| Migrations don't re-run after editing one | Goose recorded the version. Editing an applied migration is a no-op — wipe the dev dir or add a new migration. |

## The liveness check that must be right

Cleanup decides whether a directory is abandoned. **Getting this wrong deletes
a running database.** Two independent signals, and you need both:

- **Data dir:** `<dir>/postmaster.pid` exists *and* its PID is alive → in use.
- **Runtime dir:** contains a `.s.PGSQL.*` socket → in use.

Plus a **grace period** (~10 min by modification time). `go test ./...` runs
packages in parallel, so a directory created seconds ago may belong to an
instance that has not written its `postmaster.pid` yet. Without the grace
window, one package's sweep deletes another's starting database.

> **`ppid == 1` does not mean unused.** A reparented postmaster looks orphaned
> but may be adopted by a live dev server via the port-reuse path (principle
> 10). Liveness, not parentage, is the signal.

> **Do not write this check in Taskfile's built-in shell.** Task uses
> `mvdan/sh`, where the `kill -0` guard does not behave like `/bin/sh` — it
> silently reports a live process as dead and the cleanup deletes a running
> instance's data directory. Put it in a `scripts/*.sh` file and invoke it as
> `sh scripts/clean-pg.sh`. This was found the hard way.

## Killing is opt-in

A sweep should **never** kill by default. It reclaims what is provably dead,
reports what is alive, and offers a flag:

```
removed 2 dir(s), ~134 MB; 1 left in use
re-run as 'task clean:pg FORCE=1' to stop those and reclaim them
```

The alive one may be the database backing the dev server the user is looking
at right now.

## Reference files

- `references/lifecycle.go` — `Start`/`Stop` with reliable shutdown, owned-dir
  tracking, port adoption, stable binaries dir.
- `references/sweep.go` — orphan sweep with the liveness rules and grace
  period, plus its test.
- `references/testhelpers.go` — shared instance, `RunTestMain` with signal
  trapping, `TruncateAll`.
- `references/clean-pg.sh` — POSIX cleanup script, safe by default,
  `FORCE=1` to stop live instances.
- `references/taskfile.yml` — `task clean` / `task clean:pg` wiring.

## Style notes for using this skill

- Copy `references/` into `internal/embeddedpg` and `internal/testhelpers`,
  then adapt names. These are working files, not sketches.
- Wire the sweep into `Start` immediately — retrofitting it after the temp dir
  has grown to gigabytes is how people end up believing embedded Postgres is
  unreliable.
- When a user reports "tests are slow", check for a per-test instance before
  anything else.
