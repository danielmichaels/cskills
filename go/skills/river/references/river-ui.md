# River UI

River UI (https://riverqueue.com/docs/river-ui) is a web frontend for viewing and managing jobs — browse queues, inspect args/metadata/errors, retry/cancel/delete jobs — without hand-querying the database. Repo: `riverqueue.com/riverui` (`github.com/riverqueue/riverui`). OSS and Pro variants.

There are two ways to run it, and **which database you use decides which one to pick**:

| Database | How to run River UI | Why |
|---|---|---|
| **Postgres** | Standalone binary / Docker image, pointed at `DATABASE_URL` | The published `riverui` binary speaks pgx and connects over the network |
| **SQLite** | **Embed** the handler in your Go app (a few lines), then containerize that app | SQLite is an in-process file; the standalone image is Postgres-only, so the UI lives in the same process that holds the DB |

Both are easy. Embedding is arguably easier — it's ~6 lines and needs no extra service.

## Postgres — standalone Docker image

The published image takes `DATABASE_URL` (or standard `PG*` vars) and serves on port **8080**.

```bash
docker pull ghcr.io/riverqueue/riverui:latest

docker run -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/mydb?sslmode=disable" \
  ghcr.io/riverqueue/riverui:latest
```

The database must already have River's schema (`river migrate-up --database-url "$DATABASE_URL"`), or the UI won't start.

With docker-compose alongside your Postgres:

```yaml
services:
  riverui:
    image: ghcr.io/riverqueue/riverui:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: "postgres://postgres:postgres@postgres:5432/app?sslmode=disable"
    depends_on:
      postgres:
        condition: service_healthy
```

### Standalone env vars & flags (from `internal/riveruicmd/riveruicmd.go`)

| Env var | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | — | Postgres connection string. Required (or use `PG*` vars: `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`). |
| `PORT` | `8080` | Listen port. |
| `RIVER_HOST` | `localhost` | Bind host. **Set empty (`RIVER_HOST=`) to bind all interfaces** — needed inside Docker so the port is reachable. |
| `PATH_PREFIX` (or `-prefix` flag) | `/` | Serve under a sub-path, e.g. `/riverui`. Must start with `/`. |
| `RIVER_BASIC_AUTH_USER` / `RIVER_BASIC_AUTH_PASS` | — | Enable HTTP basic auth. |
| `RIVER_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error`. |
| `RIVER_LOG_FORMAT` | text | `json` for `slog.JSONHandler`. |
| `RIVER_JOB_LIST_HIDE_ARGS_BY_DEFAULT` | `false` | Hide job args in the list view by default. |
| `CORS_ORIGINS` | — | Comma-separated allowed origins. |
| `OTEL_ENABLED` | `false` | OpenTelemetry instrumentation. |

> Note: the official Docker image binds to `RIVER_HOST` (default `localhost`). If the port isn't reachable from the host, set `RIVER_HOST=` (empty) so it listens on `0.0.0.0`.

Install without Docker:

```bash
go install riverqueue.com/riverui/cmd/riverui@latest
DATABASE_URL="postgres://..." riverui     # serves on :8080
```

## SQLite — embed River UI in your app

The UI handler is generic over the River client's transaction type, so a `riversqlite`-backed client (`*river.Client[*sql.Tx]`) drops right in. Mount it on your existing HTTP mux:

```go
import (
    "database/sql"
    _ "modernc.org/sqlite"
    "github.com/riverqueue/river"
    "github.com/riverqueue/river/riverdriver/riversqlite"
    "riverqueue.com/riverui"
)

db, _ := sql.Open("sqlite", "file:app.db?_journal_mode=WAL&_busy_timeout=5000")
db.SetMaxOpenConns(1)

client, _ := river.NewClient(riversqlite.New(db), &river.Config{
    // Workers/Queues if this process also works jobs; can be insert-only too.
    PollOnly: true,
})

handler, err := riverui.NewHandler(&riverui.HandlerOpts{
    Endpoints: riverui.NewEndpoints(client, nil),
    Prefix:    "/riverui",
})
if err != nil { /* handle */ }
if err := handler.Start(ctx); err != nil { /* handle */ } // starts UI background processes, NOT an HTTP server

mux := http.NewServeMux()
mux.Handle("/riverui/", handler)   // now live at http://localhost:PORT/riverui/
// ... http.ListenAndServe(addr, mux)
```

`handler.Start(ctx)` boots the UI's internal processes; it does **not** start a listener — you mount `handler` (an `http.Handler`) on your own server. The same `NewEndpoints(client, nil)` works identically with a `riverpgxv5` client if you'd rather embed under Postgres too.

### SQLite — containerizing the embedded UI

Since the UI rides inside your app, you ship your app's container and mount the SQLite file as a volume so it persists across restarts:

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
# modernc.org/sqlite is pure Go → CGO_ENABLED=0 gives a static binary
RUN CGO_ENABLED=0 go build -o /app ./cmd/myapp

FROM gcr.io/distroless/static
COPY --from=build /app /app
VOLUME /data
ENV DATABASE_PATH=/data/app.db
EXPOSE 8080
ENTRYPOINT ["/app"]
```

```bash
docker run -p 8080:8080 -v "$PWD/data:/data" myapp
# River UI at http://localhost:8080/riverui/
```

Have your app open the DB from `DATABASE_PATH` (e.g. `file:/data/app.db?_journal_mode=WAL&_busy_timeout=5000`) and call `db.SetMaxOpenConns(1)`. If you use the CGo driver (`mattn/go-sqlite3`) instead of `modernc.org/sqlite`, build with `CGO_ENABLED=1` and a base image that has libc (e.g. `debian:slim`) rather than `distroless/static`.

## Pro variant

River UI Pro (`riverqueue.com/riverproui`, image `riverqueue.com/riverproui:latest`) adds features like workflow visualization and requires a License key to pull. Same `DATABASE_URL` model for the standalone image.

## Quick decision

- **Postgres, want a separate service?** → standalone Docker image with `DATABASE_URL` + `RIVER_HOST=`.
- **SQLite, or want zero extra services?** → embed `riverui.NewHandler` in your app and mount it on your mux; containerize your app with the DB file as a volume.
