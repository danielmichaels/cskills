---
name: huma
description:
  Huma REST API framework patterns for Go. Covers route registration, validation tags, error handling (RFC 9457), middleware, security schemes, resolvers, response streaming, auto-patch, SSE (Server-Sent Events), and testing with humatest. Triggers on: huma, API handlers, request validation, OpenAPI generation, SSE streaming, PATCH.
category: custom
---

# Huma Framework Guide

Huma is a Go REST API framework that generates OpenAPI 3.1 from code. It is router-agnostic — you bring your own router and huma layers on top.

**Request pipeline:** Router Middleware → Huma Middleware → Unmarshal → Validate → Resolve → Handler → Transform → Marshal → Response

## Handler Signature

Every handler follows one consistent signature:

```go
func(context.Context, *Input) (*Output, error)
```

Both Input and Output are always structs. Use `*struct{}` for empty inputs/outputs. Huma unmarshals, validates, and resolves the input before your handler runs. Return `nil, err` for errors, `&output, nil` for success.

## Route Registration

Use `huma.Register` with an explicit `huma.Operation` for stable operation IDs (important for SDK generation):

```go
huma.Register(api, huma.Operation{
    OperationID:   "create-thing",
    Method:        http.MethodPost,
    Path:          "/api/v1/things",
    Summary:       "Create a thing",
    Description:   "Longer explanation of what this does.",
    DefaultStatus: http.StatusCreated,
    Tags:          []string{"Things"},
    Middlewares:   huma.Middlewares{authMW},
    Security: []map[string][]string{
        {"bearerAuth": {}},
    },
}, app.handleCreateThing)
```

Key `huma.Operation` fields:
- `OperationID` — unique, used for SDK generation. Always set explicitly.
- `DefaultStatus` — override the default 200 (use 201 for creation endpoints, 204 for deletes)
- `MaxBodyBytes` — per-operation body size limit (default 1 MiB)
- `Middlewares` — per-operation middleware chain
- `Security` — references security schemes from config

Convenience shortcuts exist (`huma.Get`, `huma.Post`, etc.) but auto-generate operation IDs from the path, which makes SDK output less predictable. Prefer `huma.Register` for production APIs.

## Request Inputs

Input structs combine path/query/header/cookie parameters with a request body.

### Parameter Tags

| Tag | Purpose | Default |
|-----|---------|---------|
| `path:"name"` | Path parameter | Always required |
| `query:"name"` | Query parameter | Optional |
| `header:"Name"` | Header parameter | Optional |
| `cookie:"name"` | Cookie parameter | Optional |

```go
type GetThingInput struct {
    ID     string `path:"id"`
    Format string `query:"format" enum:"json,csv" default:"json" doc:"Response format"`
    Auth   string `header:"Authorization" required:"true"`
}
```

Supported types: `bool`, `[u]int[16/32/64]`, `float32/64`, `string`, `time.Time`, slices.

For query param slices, use `explode` for `?tags=a&tags=b` style:
```go
Tags []string `query:"tags,explode"`
```

### Request Body

The `Body` field handles structured JSON input. Body fields are **required by default** — use pointer types or `omitempty`/`omitzero` to make them optional:

```go
type CreateThingInput struct {
    Body struct {
        Name     string  `json:"name" required:"true" minLength:"1" doc:"Thing name"`
        Priority int     `json:"priority" minimum:"1" maximum:"10" default:"5"`
        Notes    *string `json:"notes,omitempty" doc:"Optional notes"`
    }
}
```

A pointer `Body` makes the entire body optional. A non-pointer `Body` is required.

For raw bytes (binary uploads, plain text):
```go
type UploadInput struct {
    RawBody []byte `contentType:"application/octet-stream"`
}
```

### Input Composition

Embed shared parameter structs to avoid repetition:

```go
type AuthParam struct {
    TransactionID string `header:"X-Transaction-ID" required:"true"`
}

type PaginationParams struct {
    Cursor string `query:"cursor"`
    Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"20"`
}

type ListThingsInput struct {
    AuthParam
    PaginationParams
}
```

## Response Outputs

```go
type CreateThingOutput struct {
    Status int        // Dynamic status code (set in handler)
    Body   ThingModel // Serialized as JSON response
}
```

- Default status is `200` for responses with bodies, `204` for empty
- Override default with `DefaultStatus` on the operation, or set `Status` field dynamically in the handler
- Use `Body []byte` for binary responses (bypasses serialization)

### Response Headers and Cookies

```go
type MyOutput struct {
    ETag        string      `header:"ETag"`
    ContentType string      `header:"Content-Type"`
    SetCookie   http.Cookie `header:"Set-Cookie"`
    Body        MyBody
}
```

## Validation Tags

Huma validates inputs automatically via struct tags before your handler runs. Failed validation returns `422 Unprocessable Entity` with detailed error locations. See `references/validation-tags.md` for the complete tag reference.

Most commonly used:

| Tag | Example |
|-----|---------|
| `required:"true"` | Field must be present |
| `minLength:"1"` | Minimum string length |
| `maxLength:"255"` | Maximum string length |
| `pattern:"^\\d{4}-\\d{2}-\\d{2}$"` | Regex validation |
| `minimum:"0"` | Minimum number value |
| `maximum:"100"` | Maximum number value |
| `enum:"a,b,c"` | Allowed values |
| `format:"email"` | Format hint (email, uuid, uri, date-time, etc.) |
| `default:"value"` | Default value |
| `doc:"description"` | OpenAPI description |
| `example:"value"` | OpenAPI example |

Prefer built-in validation tags over custom resolvers — tags auto-document in the OpenAPI spec, resolvers do not.

## Error Handling

Huma uses **RFC 9457 Problem Details** (`application/problem+json`). Response shape:

```json
{
  "status": 422,
  "title": "Unprocessable Entity",
  "detail": "validation failed",
  "errors": [{"location": "body.name", "message": "expected string", "value": true}]
}
```

### In Handlers — return huma error helpers

```go
func (app *App) handleGetThing(ctx context.Context, input *GetThingInput) (*GetThingOutput, error) {
    thing, err := app.store.GetThing(ctx, input.ID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, huma.Error404NotFound("thing not found")
        }
        return nil, huma.Error500InternalServerError("internal error")
    }
    return &GetThingOutput{Body: thing}, nil
}
```

Available error helpers:
- `huma.Error400BadRequest(msg, errs...)`
- `huma.Error401Unauthorized(msg, errs...)`
- `huma.Error403Forbidden(msg, errs...)`
- `huma.Error404NotFound(msg, errs...)`
- `huma.Error409Conflict(msg, errs...)`
- `huma.Error410Gone(msg, errs...)`
- `huma.Error500InternalServerError(msg, errs...)`
- `huma.Error501NotImplemented(msg, errs...)`
- `huma.NewError(status, msg, errs...)` — arbitrary status

For errors with response headers:
```go
return nil, huma.ErrorWithHeaders(
    huma.Error404NotFound("not found"),
    http.Header{"Cache-Control": {"no-store"}},
)
```

### In Middleware — use huma.WriteErr

Middleware cannot return errors. Instead, write the error directly and do NOT call `next()`:

```go
func (app *App) authMiddleware(api huma.API) func(ctx huma.Context, next func(huma.Context)) {
    return func(ctx huma.Context, next func(huma.Context)) {
        token := ctx.Header("Authorization")
        if token == "" {
            huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing authorization")
            return
        }
        next(ctx)
    }
}
```

## Middleware

### Huma-Native Middleware (router-agnostic)

```go
func LoggingMiddleware(ctx huma.Context, next func(huma.Context)) {
    start := time.Now()
    next(ctx)
    slog.Info("request", "path", ctx.URL().Path, "duration", time.Since(start))
}

api.UseMiddleware(LoggingMiddleware)
```

### Context Values

Inject values for handlers to consume:

```go
type contextKey int
const userContextKey contextKey = iota

func (app *App) authMiddleware(api huma.API) func(ctx huma.Context, next func(huma.Context)) {
    return func(ctx huma.Context, next func(huma.Context)) {
        user, err := app.authenticate(ctx.Header("Authorization"))
        if err != nil {
            huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid token")
            return
        }
        ctx = huma.WithValue(ctx, userContextKey, user)
        next(ctx)
    }
}
```

Retrieve in handlers via standard `context.Value`:

```go
func (app *App) handleMe(ctx context.Context, input *struct{}) (*MeOutput, error) {
    user := ctx.Value(userContextKey).(User)
    return &MeOutput{Body: user}, nil
}
```

### Per-Operation Middleware

Apply middleware selectively via the `Middlewares` field on `huma.Operation`:

```go
huma.Register(api, huma.Operation{
    OperationID: "admin-action",
    Method:      http.MethodPost,
    Path:        "/admin/action",
    Middlewares: huma.Middlewares{authMW, adminOnlyMW},
}, app.handleAdminAction)
```

## Security Schemes

Configure OpenAPI security schemes in the huma config:

```go
cfg := huma.DefaultConfig("My API", "1.0.0")
cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
    "bearerAuth": {
        Type:         "http",
        Scheme:       "bearer",
        BearerFormat: "JWT",
    },
    "apiKeyAuth": {
        Type: "apiKey",
        In:   "header",
        Name: "X-API-Key",
    },
}
```

Reference on operations:

```go
Security: []map[string][]string{
    {"bearerAuth": {}},
}
```

Multiple entries in the slice means OR (any scheme suffices). Multiple keys in one map means AND (all required).

## Resolvers

Resolvers run after built-in validation, before the handler. Use them for custom validation or to enrich the input with derived data.

```go
type MyInput struct {
    Name string `query:"name"`
}

func (m *MyInput) Resolve(ctx huma.Context) []error {
    m.Name = strings.TrimSpace(m.Name)
    if m.Name == "admin" {
        return []error{&huma.ErrorDetail{
            Message:  "reserved name",
            Location: "query.name",
            Value:    m.Name,
        }}
    }
    return nil
}

var _ huma.Resolver = (*MyInput)(nil)
```

Resolver errors automatically trigger 400-level responses. Return a `huma.StatusError` to control the status code:

```go
return []error{huma.Error403Forbidden("not allowed")}
```

## Transformers

Transformers run after the handler but before serialization. They can modify the response body.

```go
func AddMetaTransformer(ctx huma.Context, status string, v any) (any, error) {
    // modify or wrap v before it gets serialized
    return v, nil
}
```

## Testing with humatest

```go
import "github.com/danielgtaylor/huma/v2/humatest"

func TestCreateThing(t *testing.T) {
    _, api := humatest.New(t)
    app := NewApp(/* deps */)
    app.registerEndpoints(api)

    resp := api.Post("/api/v1/things",
        "X-Transaction-ID: test-123",
        map[string]any{"name": "widget"},
    )
    assert.Equal(t, http.StatusCreated, resp.Code)
}
```

String arguments become headers, any other value becomes the JSON body. Returns `*httptest.ResponseRecorder`.

## Config and Initialization

```go
func setupAPI() huma.API {
    router := chi.NewMux()
    // Router-level middleware (runs before huma)
    router.Use(middleware.Logger)

    cfg := huma.DefaultConfig("My API", "1.0.0")
    cfg.Info.Description = "API description"
    cfg.Tags = []*huma.Tag{
        {Name: "Things", Description: "Thing management"},
    }
    cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{...}

    api := humachi.New(router, cfg)
    // Huma-level middleware (runs after router middleware)
    api.UseMiddleware(loggingMW)

    return api
}
```

Import CBOR support for content negotiation:
```go
import _ "github.com/danielgtaylor/huma/v2/formats/cbor"
```

## Response Streaming

For streaming responses, return `*huma.StreamResponse` from your handler. The `Body` callback receives a `huma.Context` for writing directly:

```go
func (app *App) handleStream(ctx context.Context, input *StreamInput) (*huma.StreamResponse, error) {
    return &huma.StreamResponse{
        Body: func(ctx huma.Context) {
            ctx.SetHeader("Content-Type", "text/my-stream")
            writer := ctx.BodyWriter()

            if d, ok := writer.(interface{ SetWriteDeadline(time.Time) error }); ok {
                d.SetWriteDeadline(time.Now().Add(5 * time.Second))
            }

            writer.Write([]byte("chunk one"))
            if f, ok := writer.(http.Flusher); ok {
                f.Flush()
            }

            time.Sleep(1 * time.Second)
            writer.Write([]byte("chunk two"))
        },
    }, nil
}
```

### Streaming Rules

1. Set headers **before** writing body content via `ctx.SetHeader()`
2. Use `ctx.BodyWriter()` to get the underlying `io.Writer`
3. Cast to `http.Flusher` and call `Flush()` to send buffered data immediately
4. Use `SetWriteDeadline` to extend per-write timeouts for long-lived streams
5. For SSE specifically, prefer the `sse` package over manual streaming

## Auto Patch

The `autopatch` package auto-generates `PATCH` operations for any resource that has `GET` + `PUT` but no `PATCH`. Import `github.com/danielgtaylor/huma/v2/autopatch`.

Call `autopatch.AutoPatch(api)` **after** all routes are registered:

```go
autopatch.AutoPatch(api)
```

### Supported Patch Formats

| Content-Type | Format | Default |
|---|---|---|
| `application/merge-patch+json` | JSON Merge Patch (RFC 7386) | Yes (also used when `application/json` or no Content-Type) |
| `application/json-patch+json` | JSON Patch (RFC 6902) | No |
| `application/merge-patch+shorthand` | Shorthand Merge Patch (field paths + array append) | No |

### Conditional Requests

If the `GET` response includes `ETag` or `Last-Modified` headers, AutoPatch forwards them to the `PUT`, preventing distributed write conflicts automatically.

### Disabling Per Resource

Set `"autopatch": false` in operation metadata to exclude a resource:

```go
huma.Register(api, huma.Operation{
    OperationID: "get-thing",
    Method:      http.MethodGet,
    Path:        "/things/{id}",
    Metadata:    map[string]any{"autopatch": false},
}, app.handleGetThing)
```

## Server-Sent Events (SSE)

The `sse` package provides streaming Server-Sent Events. Import `github.com/danielgtaylor/huma/v2/sse`.

### Event Type Registration

`sse.Register` takes an `eventTypeMap` mapping event names to Go types. Each event type **must be a unique Go type** — use type aliases for shared base structs:

```go
type UserEvent struct {
    UserID   int    `json:"user_id"`
    Username string `json:"username"`
}

type UserCreatedEvent UserEvent
type UserDeletedEvent UserEvent
```

### Registering an SSE Endpoint

```go
sse.Register(api, huma.Operation{
    OperationID: "stream-events",
    Method:      http.MethodGet,
    Path:        "/events",
    Summary:     "Stream events",
}, map[string]any{
    "message":    DefaultMessage{},
    "userCreate": UserCreatedEvent{},
    "userDelete": UserDeletedEvent{},
}, func(ctx context.Context, input *struct{}, send sse.Sender) {
    send.Data(DefaultMessage{Message: "connected"})
    send.Data(UserCreatedEvent{UserID: 1, Username: "foo"})
})
```

The handler signature is `func(ctx context.Context, input *I, send sse.Sender)` — input works the same as regular Huma handlers (path/query/header params, validation).

### Sending Messages

**`send.Data(v)`** — sends data with the event type inferred from the Go type's entry in the event map. This is the common path:

```go
send.Data(UserCreatedEvent{UserID: 1, Username: "foo"})
```

**`send(sse.Message{...})`** — full control over ID and retry interval. The event type is still inferred from `Data`'s Go type:

```go
send(sse.Message{
    ID:    5,
    Retry: 1000,
    Data:  UserCreatedEvent{UserID: 1, Username: "foo"},
})
```

### SSE Message Fields

| Field | Type | Purpose |
|-------|------|---------|
| `ID` | `int` | Event ID (sent as `id:` line, omitted when 0) |
| `Data` | `any` | Payload, JSON-encoded (type determines `event:` name) |
| `Retry` | `int` | Client reconnect interval in ms (sent as `retry:` line, omitted when 0) |

### Wire Format

The `"message"` event name is the SSE default — Huma omits the `event:` line for it. All other event names are sent explicitly. Data is always JSON-encoded.

### Configuration

`sse.WriteTimeout` controls the per-write deadline (default 5s). Set it before registering endpoints:

```go
sse.WriteTimeout = 10 * time.Second
```

### SSE Rules

1. Each event type in the map must be a **distinct Go type** — reuse via type alias (`type X Y`)
2. The event name is determined by the Go type of `Data`, not by a string field
3. Flushing is automatic if the adapter's `BodyWriter` implements `http.Flusher`
4. The response content type is `text/event-stream` with a `200` status
5. SSE endpoints generate proper OpenAPI schemas with `oneOf` for each event type

## Key Rules

1. Body fields are **required by default** — use `omitempty` or pointer types for optional fields
2. Unknown JSON fields are **rejected by default** (`additionalProperties: false`)
3. Path parameters are **always required** — no way to make them optional
4. Default body limit is **1 MiB** — set `MaxBodyBytes` for larger payloads
5. Prefer built-in validation tags over resolvers — tags appear in the OpenAPI spec
6. Return all validation failures at once, not one at a time
7. Use `huma.WriteErr` in middleware (not `huma.ErrorXXX` helpers — those are for handlers)
8. `huma.ModelValidator` is NOT goroutine-safe — use `huma.Validate` for concurrent use

## Reference

For full API docs (types, functions, interfaces), search https://pkg.go.dev/github.com/danielgtaylor/huma/v2 as needed.