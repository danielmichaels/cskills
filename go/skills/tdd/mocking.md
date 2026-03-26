# When to Mock (Go)

Mock at **system boundaries** only:

- External APIs (payment, email, NATS subjects you don't own)
- Time (`time.Now` — inject a clock)
- Randomness (`rand` — inject a source)
- File system (sometimes — prefer `testing/fstest.MapFS` or temp dirs)

**Don't mock:**

- Your own packages/types
- Internal collaborators
- Anything you control
- The database — use a real DuckDB instance with seed data

## Designing for Mockability in Go

### 1. Use small interfaces at consumption site

```go
// Define the interface where it's used, not where it's implemented
type Publisher interface {
    Publish(subject string, data []byte) error
}

func NotifyUser(ctx context.Context, pub Publisher, userID string, msg []byte) error {
    return pub.Publish("notify."+userID, msg)
}

// In tests:
type mockPublisher struct {
    published []struct{ subject string; data []byte }
}

func (m *mockPublisher) Publish(subject string, data []byte) error {
    m.published = append(m.published, struct{ subject string; data []byte }{subject, data})
    return nil
}
```

### 2. Prefer SDK-style interfaces over generic ones

```go
// GOOD: Each method is independently testable
type UserService interface {
    GetUser(ctx context.Context, id string) (User, error)
    ListUsers(ctx context.Context, filter Filter) ([]User, error)
    CreateUser(ctx context.Context, u User) error
}

// BAD: Generic interface requires conditional logic in mocks
type API interface {
    Do(ctx context.Context, method, path string, body any) (any, error)
}
```

### 3. Inject time and randomness

```go
// Testable: clock is injected
type Scheduler struct {
    now func() time.Time
}

func NewScheduler() *Scheduler {
    return &Scheduler{now: time.Now}
}

// In tests:
s := &Scheduler{now: func() time.Time {
    return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
}}
```

### 4. For NATS: use real embedded server in tests when possible

Prefer `natsserver.RunDefaultServer()` in tests over mocking the NATS client. This tests real message flow. Only mock NATS when testing error handling paths.

## SQL Testing: No Mocks

Never mock SQL queries. Use a real DuckDB instance:

```go
func TestGetActiveUsers(t *testing.T) {
    is := is.New(t)
    db := testutils.NewTestDB(t)

    // Seed minimal data
    _, err := db.Exec(`INSERT INTO users (id, name, active) VALUES (1, 'Alice', true), (2, 'Bob', false)`)
    is.NoErr(err)

    users, err := GetActiveUsers(context.Background(), db, time.Time{})
    is.NoErr(err)
    is.Equal(len(users), 1)
    is.Equal(users[0].Name, "Alice")
}
```
