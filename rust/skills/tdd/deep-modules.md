# Deep Modules (Rust)

## Core Principle

A **deep module** has a small interface and lots of implementation. Expose minimal public functions and types while concealing substantial underlying logic.

The opposite — **shallow modules** with extensive interfaces but minimal functionality — creates unnecessary cognitive burden without corresponding benefit.

## In Rust

```rust
// DEEP: Small interface, rich behavior behind it
pub struct Store { /* ... */ }

impl Store {
    pub fn new(db: Connection) -> Self { /* ... */ }
    pub fn save_user(&self, user: User) -> Result<(), Error> { /* ... */ }
    pub fn get_user(&self, id: &str) -> Result<User, Error> { /* ... */ }
}

// SHALLOW: Every internal step is exposed
pub struct Store { /* ... */ }

impl Store {
    pub fn new(db: Connection) -> Self { /* ... */ }
    pub fn validate_user(&self, user: &User) -> Result<(), Error> { /* ... */ }
    pub fn serialize_user(&self, user: &User) -> Result<Vec<u8>, Error> { /* ... */ }
    pub fn insert_row(&self, table: &str, data: &[u8]) -> Result<(), Error> { /* ... */ }
    pub fn deserialize_user(&self, data: &[u8]) -> Result<User, Error> { /* ... */ }
    pub fn select_row(&self, table: &str, id: &str) -> Result<Vec<u8>, Error> { /* ... */ }
}
```

## Evaluation Questions

When designing a module's public surface, ask:

1. Can I reduce the number of `pub` functions/types?
2. Can I simplify function signatures (fewer parameters)?
3. Can I encapsulate more complexity internally (private helpers in `impl` blocks)?

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
