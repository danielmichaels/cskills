---
name: woodpecker
description: Woodpecker CI v2 schema, common gotchas, and patterns for self-hosted pipelines. Use when writing or debugging .woodpecker.yml.
category: custom
---

# Woodpecker CI

Self-hosted CI. Config lives at `.woodpecker.yml` in the repo root.

## Schema (v2)

- NO `version:` key, NO top-level `volumes:`, NO top-level `secrets:`
- Top-level key is `steps:` (not `pipeline:`)
- Parallelism: `depends_on: []` on sibling steps (empty = run in parallel)
- Secrets inline with `from_secret:` inside `environment:` or `settings:`
- Named volumes declared implicitly by use in step `volumes:` — Woodpecker creates them automatically

```yaml
steps:
  build:
    depends_on: []
    image: golang:1.25-alpine
    environment:
      GOMODCACHE: /woodpecker/go/mod
      MY_SECRET:
        from_secret: my_secret_name
    volumes:
      - my_cache:/woodpecker/cache
    commands:
      - go build ./...
    when:
      event: [push, pull_request, manual]
```

## Event triggers

Valid `when: event:` values: `push`, `pull_request`, `tag`, `manual`, `cron`.

Always include `manual` so the Woodpecker UI Restart button works:

```yaml
when:
  event: [push, pull_request, manual]
```

For steps that should only run on main (docker build, deploy):

```yaml
when:
  event: [push, manual]
  branch: main
```

## Step ordering

```yaml
steps:
  test:
    depends_on: []   # runs in parallel with lint
  lint:
    depends_on: []   # runs in parallel with test
  deploy:
    depends_on: [test, lint]   # waits for both
```

## Common gotchas

### `Additional property version/volumes/secrets is not allowed`

This is the v1 schema. Remove `version:`, top-level `volumes:`, and top-level `secrets:`. Move secrets inline with `from_secret:`.

### `Insufficient trust level to use volumes`

Repo must be marked **Trusted** in Woodpecker admin → repo settings → Trust.

### `bad_habit: no when/event filter`

Every step needs a `when: event:` filter. Add at minimum:
```yaml
when:
  event: [push, pull_request]
```

### `Duplicate mount point` with golang image

`golang:X-alpine` declares `VOLUME /go` — Docker rejects named volume mounts at subpaths of image-declared volumes. Redirect caches outside `/go`:

```yaml
environment:
  GOMODCACHE: /woodpecker/go/mod
  GOCACHE: /woodpecker/go/build
volumes:
  - go_mod:/woodpecker/go/mod
  - go_build:/woodpecker/go/build
```

### `initdb: cannot be run as root`

Embedded postgres (or any tool that runs `initdb`) refuses to start as root. Run tests as a non-root user:

```yaml
test:
  image: golang:1.25   # Debian, NOT alpine — glibc vs musl matters for embedded postgres
  volumes:
    - go_mod:/woodpecker/go/mod
    - go_build:/woodpecker/go/build
    - embedded_pg:/home/ci/.embedded-postgres-go
  commands:
    - GOMODCACHE=/woodpecker/go/mod go mod download
    - useradd -m ci
    - chown -R ci:ci /woodpecker/go /home/ci
    - |
      cat > /tmp/run-tests.sh << EOF
      #!/bin/sh
      set -e
      export HOME=/home/ci
      export PATH=$PATH
      export GOMODCACHE=/woodpecker/go/mod
      export GOCACHE=/woodpecker/go/build
      cd $CI_WORKSPACE/go
      exec go test -race -p 1 ./...
      EOF
    - chmod +x /tmp/run-tests.sh
    - su ci /tmp/run-tests.sh
```

Key points:
- Mount the embedded postgres cache under the ci user's home (`/home/ci/.embedded-postgres-go`), NOT `/root/`
- `$PATH` and `$CI_WORKSPACE` expand at heredoc-write time (as root), baking the correct values into the script
- Use Debian image (`golang:1.25`) not Alpine for glibc compatibility

## Manual pipeline only runs `clone`

Manual pipelines fire event `manual`, not `push`. Steps without `manual` in their `when: event:` list are skipped entirely. Add `manual` to every step's event list.

## Retrigger pipeline

```bash
git commit --allow-empty -m "ci: retrigger"
git push
```

Or use the Woodpecker UI Restart button (only works after adding `manual` to event filters).

## CI_* environment variables

| Variable | Value |
|---|---|
| `CI_WORKSPACE` | Absolute path to cloned repo (e.g. `/woodpecker/src/github.com/owner/repo`) |
| `CI_COMMIT_SHA` | Full commit SHA |
| `CI_REPO_NAME` | Repo name |
| `CI_BRANCH` | Branch name |
| `CI_BUILD_EVENT` | Event that triggered the pipeline (`push`, `pull_request`, `manual`, etc.) |

## Docker plugin

```yaml
docker:
  image: woodpeckerci/plugin-docker-buildx
  settings:
    registry: ghcr.io
    repo: ghcr.io/owner/repo
    username:
      from_secret: ghcr_username
    password:
      from_secret: ghcr_token
    dockerfile: Dockerfile
    context: .
    tags:
      - latest
      - ${CI_COMMIT_SHA}
  when:
    event: [push, manual]
    branch: main
  depends_on: [test, lint]
```
