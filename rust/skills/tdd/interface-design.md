# Interface Design for Testability (Rust)

Good traits make testing natural.

## 1. Accept dependencies, don't create them

```rust
// Testable: dependency injected
fn process_order(order: Order, gateway: &dyn PaymentGateway) -> Result<(), Error> {
    gateway.charge(order.total)
}

// Hard to test: dependency created internally
fn process_order(order: Order) -> Result<(), Error> {
    let gateway = StripeClient::new(&std::env::var("STRIPE_KEY")?);
    gateway.charge(order.total)
}
```

## 2. Return results, don't produce side effects

```rust
// Testable: returns a value
fn calculate_discount(cart: &Cart) -> Discount {
    // ...
}

// Hard to test: mutates input
fn apply_discount(cart: &mut Cart) {
    cart.total -= compute_discount(cart);
}
```

## 3. Use Rust traits at boundaries

```rust
// Define small traits where you consume them
trait PaymentGateway {
    fn charge(&self, amount: u64) -> Result<(), Error>;
}

// Accept &dyn Trait or generic T: Trait
fn checkout(cart: Cart, charger: &dyn PaymentGateway) -> Result<Receipt, Error> {
    // ...
}

// Or with generics (zero-cost, monomorphized):
fn checkout<G: PaymentGateway>(cart: Cart, charger: &G) -> Result<Receipt, Error> {
    // ...
}
```

## 4. Small surface area

- Fewer `pub` functions = fewer tests needed
- Fewer parameters = simpler test setup
- Prefer builder pattern (`Default` + setter methods) over long parameter lists

## 5. For SQL queries

```rust
// Testable: query function takes a connection reference
trait Querier {
    fn query(&self, sql: &str, params: &[Value]) -> Result<Rows, Error>;
}

fn get_active_users(q: &dyn Querier, since: DateTime<Utc>) -> Result<Vec<User>, Error> {
    // ...
}

// In tests: pass a real DuckDB connection with seed data
// In production: pass the actual Connection
```
