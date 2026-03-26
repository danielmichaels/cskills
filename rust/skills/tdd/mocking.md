# When to Mock (Rust)

Mock at **system boundaries** only:

- External APIs (payment, email, NATS subjects you don't own)
- Time (`chrono::Utc::now()` — inject a clock)
- Randomness (`rand` — inject a source)
- File system (sometimes — prefer `tempfile` crate or `std::env::temp_dir()`)

**Don't mock:**

- Your own types/modules
- Internal collaborators
- Anything you control
- The database — use a real DuckDB instance with seed data

## Designing for Mockability in Rust

### 1. Use small traits at consumption site

```rust
// Define the trait where it's used, not where it's implemented
trait Publisher {
    fn publish(&self, subject: &str, data: &[u8]) -> Result<(), Error>;
}

fn notify_user(pub_: &dyn Publisher, user_id: &str, msg: &[u8]) -> Result<(), Error> {
    pub_.publish(&format!("notify.{}", user_id), msg)
}

// In tests: manual stub
struct MockPublisher {
    published: std::cell::RefCell<Vec<(String, Vec<u8>)>>,
}

impl Publisher for MockPublisher {
    fn publish(&self, subject: &str, data: &[u8]) -> Result<(), Error> {
        self.published.borrow_mut().push((subject.to_string(), data.to_vec()));
        Ok(())
    }
}
```

Or use `mockall` for automatic mock generation:

```rust
use mockall::automock;

#[automock]
trait Publisher {
    fn publish(&self, subject: &str, data: &[u8]) -> Result<(), Error>;
}
```

### 2. Prefer SDK-style traits over generic ones

```rust
// GOOD: Each method is independently testable
trait UserService {
    fn get_user(&self, id: &str) -> Result<User, Error>;
    fn list_users(&self, filter: &Filter) -> Result<Vec<User>, Error>;
    fn create_user(&self, user: User) -> Result<(), Error>;
}

// BAD: Generic trait requires conditional logic in mocks
trait Api {
    fn do_request(&self, method: &str, path: &str, body: Option<Value>) -> Result<Value, Error>;
}
```

### 3. Inject time and randomness

```rust
// Testable: clock injected as a closure or trait
struct Scheduler {
    now: Box<dyn Fn() -> DateTime<Utc>>,
}

impl Scheduler {
    pub fn new() -> Self {
        Self { now: Box::new(|| chrono::Utc::now()) }
    }
}

// In tests:
let s = Scheduler {
    now: Box::new(|| {
        chrono::DateTime::parse_from_rfc3339("2025-01-01T00:00:00Z")
            .unwrap()
            .with_timezone(&chrono::Utc)
    }),
};
```

### 4. For NATS: use real embedded server in tests when possible

Prefer a real NATS embedded test server in tests over mocking the NATS client. This tests real message flow. Only mock NATS when testing error handling paths.

## SQL Testing: No Mocks

Never mock SQL queries. Use a real DuckDB instance:

```rust
#[test]
fn test_get_active_users() {
    let db = test_db();

    db.execute(
        "INSERT INTO users (id, name, active) VALUES (1, 'Alice', true), (2, 'Bob', false)",
        [],
    ).unwrap();

    let users = get_active_users(&db, DateTime::default()).unwrap();
    assert_eq!(users.len(), 1);
    assert_eq!(users[0].name, "Alice");
}
```
