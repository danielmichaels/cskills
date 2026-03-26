# Deep Modules (Go)

## Core Principle

A **deep module** has a small interface and lots of implementation. Expose minimal exported functions and types while concealing substantial underlying logic.

The opposite — **shallow modules** with extensive interfaces but minimal functionality — creates unnecessary cognitive burden without corresponding benefit.

## In Go

```go
// DEEP: Small interface, rich behavior behind it
type Store struct { ... }

func NewStore(db *sql.DB) *Store { ... }
func (s *Store) SaveUser(ctx context.Context, u User) error { ... }
func (s *Store) GetUser(ctx context.Context, id string) (User, error) { ... }

// SHALLOW: Every internal step is exposed
type Store struct { ... }

func NewStore(db *sql.DB) *Store { ... }
func (s *Store) ValidateUser(u User) error { ... }
func (s *Store) SerializeUser(u User) ([]byte, error) { ... }
func (s *Store) InsertRow(ctx context.Context, table string, data []byte) error { ... }
func (s *Store) DeserializeUser(data []byte) (User, error) { ... }
func (s *Store) SelectRow(ctx context.Context, table string, id string) ([]byte, error) { ... }
```

## Evaluation Questions

When designing a package's exported surface, ask:

1. Can I reduce the number of exported functions/types?
2. Can I simplify function signatures (fewer parameters)?
3. Can I encapsulate more complexity internally?

## In SQL

```sql
-- DEEP: A view that encapsulates complex joins
CREATE VIEW hx_consumer AS
  SELECT ... FROM consumer_stats
  JOIN consumer_ident USING (...)
  JOIN consumer_opts USING (...);

-- Callers just: SELECT * FROM hx_consumer WHERE ...

-- SHALLOW: Callers must know and repeat the join logic
SELECT s.*, i.name, o.ack_policy
FROM consumer_stats s
JOIN consumer_ident i ON ...
JOIN consumer_opts o ON ...
WHERE ...;
```

Views, CTEs, and well-named columns act as deep interfaces in SQL — they hide join complexity behind a simple `SELECT`.
