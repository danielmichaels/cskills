// Package testhelpers provides a shared embedded Postgres for service
// tests: one instance per test binary, migrated once, tables cleared
// between tests.
package testhelpers

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/danielmichaels/watch-me-grow/internal/embeddedpg"
	"github.com/danielmichaels/watch-me-grow/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	emb     *embeddedpg.Server
	Pool    *pgxpool.Pool
	Queries *store.Queries
	DSN     string
}

var (
	shared *Postgres
	mu     sync.Mutex
)

// Shared returns the per-binary embedded Postgres, starting and migrating
// it on first call.
func Shared(ctx context.Context, t *testing.T) *Postgres {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if shared == nil {
		pg, err := create(ctx)
		if err != nil {
			t.Fatalf("start shared test postgres: %v", err)
		}
		shared = pg
	}
	return shared
}

func create(ctx context.Context) (*Postgres, error) {
	srv, err := embeddedpg.Start(embeddedpg.Options{})
	if err != nil {
		return nil, fmt.Errorf("embeddedpg.Start: %w", err)
	}
	if err := store.MigrateUp(srv.DSN, nil); err != nil {
		srv.Stop() //nolint:errcheck
		return nil, fmt.Errorf("MigrateUp: %w", err)
	}
	cfg, err := pgxpool.ParseConfig(srv.DSN)
	if err != nil {
		srv.Stop() //nolint:errcheck
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	cfg.MaxConnLifetime = 3 * time.Minute
	cfg.MaxConnIdleTime = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		srv.Stop() //nolint:errcheck
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return &Postgres{emb: srv, Pool: pool, Queries: store.New(pool), DSN: srv.DSN}, nil
}

func cleanup() {
	mu.Lock()
	defer mu.Unlock()
	if shared != nil {
		shared.Pool.Close()
		shared.emb.Stop() //nolint:errcheck
		shared = nil
	}
}

// RunTestMain runs m and cleans up the shared Postgres, including on
// SIGINT/SIGTERM so an interrupted run doesn't leave the embedded-postgres
// process orphaned. SIGKILL and go test's -timeout force-exit bypass user
// code; `pkill -f embeddedpg-data-` is the backstop there.
func RunTestMain(m *testing.M) int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			cleanup()
			os.Exit(1)
		case <-done:
		}
	}()

	code := m.Run()
	close(done)
	cleanup()
	return code
}

// TruncateAll clears every application table for test isolation. River's
// tables are left alone: job state assertions use rivertest against the
// same DB, and truncating mid-suite would race its maintenance services.
func (p *Postgres) TruncateAll(ctx context.Context, t *testing.T) {
	t.Helper()
	_, err := p.Pool.Exec(ctx, `TRUNCATE users, magic_links, sessions, boards, board_memberships, media, board_invites RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}
