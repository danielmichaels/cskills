# Interface Design for Testability (Go)

Good interfaces make testing natural.

## 1. Accept dependencies, don't create them

```go
// Testable: dependency injected
func ProcessOrder(ctx context.Context, order Order, gateway PaymentGateway) error {
    return gateway.Charge(ctx, order.Total)
}

// Hard to test: dependency created internally
func ProcessOrder(ctx context.Context, order Order) error {
    gateway := stripe.NewClient(os.Getenv("STRIPE_KEY"))
    return gateway.Charge(ctx, order.Total)
}
```

## 2. Return results, don't produce side effects

```go
// Testable: returns a value
func CalculateDiscount(cart Cart) Discount {
    // ...
}

// Hard to test: mutates input
func ApplyDiscount(cart *Cart) {
    cart.Total -= computeDiscount(cart)
}
```

## 3. Use Go interfaces at boundaries

```go
// Define small interfaces where you consume them
type PaymentCharger interface {
    Charge(ctx context.Context, amount int) error
}

// Accept the interface, not the concrete type
func Checkout(ctx context.Context, cart Cart, charger PaymentCharger) (Receipt, error) {
    // ...
}
```

## 4. Small surface area

- Fewer exported functions = fewer tests needed
- Fewer parameters = simpler test setup
- Prefer `Options` structs or functional options over long parameter lists

## 5. For SQL queries

```go
// Testable: query function takes a querier interface
type Querier interface {
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func GetActiveUsers(ctx context.Context, q Querier, since time.Time) ([]User, error) {
    // ...
}

// In tests: pass a real DuckDB connection with seed data
// In production: pass the actual *sql.DB
```
