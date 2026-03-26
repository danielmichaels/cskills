# Good and Bad Tests (Rust & SQL)

## Good Tests

**Integration-style**: Test through real interfaces, not mocks of internal parts.

```rust
// GOOD: Tests observable behavior through the public API
#[test]
fn checkout_valid_cart() {
    let cart = Cart::new();
    cart.add(product);
    let result = checkout(&cart, &test_payment());

    assert!(result.is_ok());
    assert_eq!(result.unwrap().status, Status::Confirmed);
}
```

Characteristics:

- Tests behavior callers care about
- Uses public API only
- Survives internal refactors
- Describes WHAT, not HOW
- One logical assertion per test (or tightly related group)
- Uses `assert_eq!`, `assert!`, `assert_ne!` — or `pretty_assertions` for richer diffs

## Bad Tests

**Implementation-detail tests**: Coupled to internal structure.

```rust
// BAD: Tests implementation details
#[test]
fn checkout_calls_payment_process() {
    let mut mock = MockPayment::new();
    mock.expect_charge().times(1).returning(|_| Ok(()));

    let _ = checkout(&cart, &mock);
    mock.checkpoint(); // asserting call count
}
```

Red flags:

- Mocking internal collaborators
- Testing private functions directly
- Asserting on call counts or call order
- Test breaks when refactoring without behavior change
- Test name describes HOW not WHAT
- Verifying through external means instead of the interface

## Verify Through the Interface

```rust
// BAD: Bypasses interface to verify via raw SQL
#[test]
fn create_user_saves_to_db() {
    let store = Store::new(&db);
    let _ = store.create_user(User { name: "Alice".into() });

    let name: String = db
        .query_row("SELECT name FROM users WHERE name = 'Alice'", [], |r| r.get(0))
        .unwrap();
    assert_eq!(name, "Alice");
}

// GOOD: Verifies through the same interface
#[test]
fn create_user_is_retrievable() {
    let store = Store::new(&db);

    let created = store.create_user(User { name: "Alice".into() }).unwrap();
    let retrieved = store.get_user(&created.id).unwrap();

    assert_eq!(retrieved.name, "Alice");
}
```

## SQL Query Tests

```rust
// GOOD: Minimal seed data, tests the query's behavior
#[test]
fn get_top_accounts_ranks_correctly() {
    let db = test_db();
    seed_account_stats(&db, &[
        AccountStat { account: "a", bytes: 100 },
        AccountStat { account: "b", bytes: 300 },
        AccountStat { account: "c", bytes: 200 },
    ]);

    let accounts = get_top_accounts(&db, 2).unwrap();
    assert_eq!(accounts.len(), 2);
    assert_eq!(accounts[0].account, "b"); // highest first
    assert_eq!(accounts[1].account, "c");
}
```

## Table-Driven Tests

Use `rstest` for parameterized tests:

```rust
use rstest::rstest;

#[rstest]
#[case("100B", 100, false)]
#[case("2KB", 2048, false)]
#[case("abc", 0, true)]
fn parse_size(#[case] input: &str, #[case] want: i64, #[case] should_err: bool) {
    let result = parse_size(input);
    if should_err {
        assert!(result.is_err());
    } else {
        assert_eq!(result.unwrap(), want);
    }
}
```

Or a manual loop for simpler cases:

```rust
#[test]
fn parse_size_cases() {
    let cases = vec![
        ("100B", 100i64, false),
        ("2KB", 2048, false),
        ("abc", 0, true),
    ];
    for (input, want, should_err) in cases {
        let result = parse_size(input);
        if should_err {
            assert!(result.is_err(), "input={input}");
        } else {
            assert_eq!(result.unwrap(), want, "input={input}");
        }
    }
}
```
