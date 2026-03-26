# Good and Bad Tests (Go & SQL)

## Good Tests

**Integration-style**: Test through real interfaces, not mocks of internal parts.

```go
// GOOD: Tests observable behavior through the public API
func TestCheckout_ValidCart(t *testing.T) {
    is := is.New(t)

    cart := NewCart()
    cart.Add(product)
    result, err := Checkout(ctx, cart, testPayment)

    is.NoErr(err)
    is.Equal(result.Status, StatusConfirmed)
}
```

Characteristics:

- Tests behavior callers care about
- Uses exported API only
- Survives internal refactors
- Describes WHAT, not HOW
- One logical assertion per test (or tightly related group)
- Uses `is` package for concise assertions
- Uses `t.Helper()` in helper functions

## Bad Tests

**Implementation-detail tests**: Coupled to internal structure.

```go
// BAD: Tests implementation details
func TestCheckout_CallsPaymentProcess(t *testing.T) {
    mock := &mockPayment{}
    _ = Checkout(ctx, cart, mock)

    if mock.callCount != 1 {
        t.Fatal("expected exactly one call to Process")
    }
    if mock.lastAmount != cart.Total {
        t.Fatal("expected charge for cart total")
    }
}
```

Red flags:

- Mocking internal collaborators
- Testing unexported functions
- Asserting on call counts or call order
- Test breaks when refactoring without behavior change
- Test name describes HOW not WHAT
- Verifying through external means instead of the interface

## Verify Through the Interface

```go
// BAD: Bypasses interface to verify via raw SQL
func TestCreateUser_SavesToDB(t *testing.T) {
    is := is.New(t)
    store := NewStore(db)

    _ = store.CreateUser(ctx, User{Name: "Alice"})

    var name string
    err := db.QueryRow("SELECT name FROM users WHERE name = 'Alice'").Scan(&name)
    is.NoErr(err)
    is.Equal(name, "Alice")
}

// GOOD: Verifies through the same interface
func TestCreateUser_IsRetrievable(t *testing.T) {
    is := is.New(t)
    store := NewStore(db)

    created, err := store.CreateUser(ctx, User{Name: "Alice"})
    is.NoErr(err)

    retrieved, err := store.GetUser(ctx, created.ID)
    is.NoErr(err)
    is.Equal(retrieved.Name, "Alice")
}
```

## SQL Query Tests

```go
// GOOD: Minimal seed data, tests the query's behavior
func TestGetTopAccounts_RanksCorrectly(t *testing.T) {
    is := is.New(t)
    db := testutils.NewTestDB(t)

    // Seed only what matters for this test
    seedAccountStats(t, db, []accountStat{
        {account: "a", bytes: 100},
        {account: "b", bytes: 300},
        {account: "c", bytes: 200},
    })

    accounts, err := GetTopAccounts(ctx, db, 2)
    is.NoErr(err)
    is.Equal(len(accounts), 2)
    is.Equal(accounts[0].Account, "b") // highest first
    is.Equal(accounts[1].Account, "c")
}
```

## Table-Driven Tests

Use when testing multiple cases of the same behavior:

```go
func TestParseSize(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  int64
        err   bool
    }{
        {"bytes", "100B", 100, false},
        {"kilobytes", "2KB", 2048, false},
        {"invalid", "abc", 0, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            is := is.New(t)
            got, err := ParseSize(tt.input)
            if tt.err {
                is.True(err != nil)
            } else {
                is.NoErr(err)
                is.Equal(got, tt.want)
            }
        })
    }
}
```
