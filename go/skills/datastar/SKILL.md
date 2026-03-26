---
name: datastar
description:
  Best practices and guidance for building web applications with Datastar, a hypermedia framework. Use when creating reactive UIs with backend-driven state, SSE streaming, or data-* attribute-based interactivity. Triggers on: datastar, hypermedia ui, sse streaming, data-signals.
category: custom
---

# Datastar Skill

Reference for Datastar usage in Go web applications. Datastar is a hypermedia framework combining
backend-driven reactivity (SSE) with frontend declarative attributes (`data-*`). Uses the
`datastar-go` SDK directly with Templ for server-side rendering.

## Core Concepts

- **Backend-driven SSE**: Server sends HTML fragments via Server-Sent Events using `datastar-go`
- **`data-*` attributes**: Declare behavior on HTML elements — no client-side framework needed
- **SSE event types**: `datastar-patch-elements` (DOM morphing) and `datastar-patch-signals` (reactive data)
- **Templ generates HTML**: Server-side rendering with Templ components; Datastar morphs them client-side
- **SSEBroker**: Custom pub/sub hub (`internal/ui/sse.go`) fans out pre-rendered HTML to browser subscribers
- **NATS → SSE pipeline**: Registry events → render Templ to buffer → `Broker.Publish()` → SSE stream

## Application Architecture

### SSEBroker (`internal/ui/sse.go`)

Scoped pub/sub hub that distributes pre-rendered HTML fragments to all connected browsers.

```go
type SSEBroker struct {
subscribers map[string]map[string]chan SSEEvent // scope → subID → chan
}

// Subscribe to a scope (e.g. "devices" or a specific device ID)
id, events := broker.Subscribe("devices")
defer broker.Unsubscribe("devices", id)

// Publish pre-rendered HTML to all subscribers of a scope
broker.Publish("devices", SSEEvent{Name: "device-update", HTML: buf.String()})
```

Non-blocking publish — drops events if subscriber buffer (32) is full.

### NATS → SSE Flow (`runDevicePublisher`)

```
Registry.Subscribe() → device online/offline event
  → deviceListSnapshot() (query DB + registry)
  → Templ render to bytes.Buffer
  → Broker.Publish(scope, SSEEvent{...})
  → sseStream handler fans out to all connected browsers
```

### Go SDK Usage (`datastar-go`)

All Datastar responses start with `datastar.NewSSE(w, r)`:

```go
sse := datastar.NewSSE(w, r)

// Patch a Templ component (default: outer morph by element ID)
sse.PatchElementTempl(templates.SomeComponent(data))

// Patch with options
sse.PatchElementTempl(component, datastar.WithSelector("#target-id"))

// Patch raw HTML string
sse.PatchElements(htmlString)

// Remove an element
sse.PatchElements("", datastar.WithModeRemove(), datastar.WithSelector("#element-id"))

// Append to a container (e.g. toast stack)
sse.PatchElementTempl(component, datastar.WithModeAppend(), datastar.WithSelector("#toast-stack"))
```

### Key Files

| File                                          | Role                                                    |
|-----------------------------------------------|---------------------------------------------------------|
| `internal/ui/sse.go`                          | SSEBroker pub/sub hub, SSEEvent type, patchTempl helper |
| `internal/ui/handlers.go`                     | All UI route handlers (stream, data, actions)           |
| `internal/ui/templates/*.templ`               | Templ components for all pages and fragments            |
| `internal/ui/templates/shared.templ`          | ContentSkeleton, ContentError, shared UI components     |
| `internal/ui/templates/device_layout.templ`   | DeviceLayout shell, subnav, DeviceStatusOOB             |
| `internal/ui/templates/devices.templ`         | Device list page, DeviceUpdateEvent, stats cards        |
| `internal/ui/templates/device_overview.templ` | Overview page with lazy-loaded content                  |
| `internal/server/natsutil/registry.go`        | Device discovery, Subscribe() for device events         |
| `assets/js/datastar.js`                       | Datastar client library                                 |

## Handler Patterns

### 1. SSE Stream (long-lived connection)

Used for real-time updates. Browser opens persistent SSE connection.

```go
func (h *Handlers) sseStream(w http.ResponseWriter, r *http.Request) {
sse := datastar.NewSSE(w, r)
id, events := h.Broker.Subscribe("devices")
defer h.Broker.Unsubscribe("devices", id)

// Send initial state
h.writeDevicesHello(r.Context(), sse)

keepAlive := time.NewTicker(30 * time.Second)
defer keepAlive.Stop()

for {
select {
case <-r.Context().Done():
return
case <-keepAlive.C:
sse.PatchElementTempl(templates.NavbarOnlineCountOOB(count))
case event, ok := <-events:
if !ok { return }
sse.PatchElements(event.HTML)
}
}
}
```

Template trigger: `data-init="@get('/ui/devices/stream')"` or `data-on-intersect__once`.

### 2. One-shot Data Endpoint

Fetches data via NATS, returns a single Templ fragment, then closes.

```go
func (h *Handlers) aliasesData(w http.ResponseWriter, r *http.Request) {
sse := datastar.NewSSE(w, r)
resp, err := natsutil.Request[...](r.Context(), h.Client, subject, req)
if err != nil {
sse.PatchElementTempl(templates.ContentError(dataURL, "Device is not responding."))
return
}
sse.PatchElementTempl(templates.AliasesContent(device, *resp.Data))
}
```

### 3. Action Handler — Delete with Remove

```go
func (h *Handlers) aliasDelete(w http.ResponseWriter, r *http.Request) {
// ... perform NATS request to delete ...
h.Client.InvalidateCache(subject)
sse := datastar.NewSSE(w, r)
sse.PatchElements("", datastar.WithModeRemove(), datastar.WithSelector("#alias-row-"+id))
}
```

### 4. Action Handler — Toast Notification

```go
sse.PatchElementTempl(toast.Toast(toast.Props{
Variant: toast.VariantError,
Title:   "Service action failed",
// ...
}), datastar.WithModeAppend(), datastar.WithSelector("#toast-stack"))
```

### 5. Action Handler — Update with Row Replace

```go
func (h *Handlers) approveDevice(w http.ResponseWriter, r *http.Request) {
device, _ := h.Queries.ApproveDevice(r.Context(), deviceID)
sse := datastar.NewSSE(w, r)
sse.PatchElementTempl(templates.DeviceListRow(view)) // morphs by element ID
}
```

## Template Patterns

### Lazy Loading with Skeleton

Page shell renders immediately with a skeleton placeholder. Content is fetched via `data-on-intersect__once`
when the element scrolls into view.

```templ
templ OverviewPage(device DeviceDetailView) {
    @DeviceLayout(device, "overview", "") {
        <div
            id="page-content"
            data-on-intersect__once={ "@get('/ui/devices/" + device.ID + "/data/overview')" }
        >
            @ContentSkeleton()
        </div>
    }
}
```

The data endpoint replaces `#page-content` with the real content via `PatchElementTempl`.

### SSE Stream Initialization

Device detail pages open a persistent SSE stream for real-time status updates:

```templ
<div
    class="mt-2 relative"
    data-on-intersect__once={ "@get('/ui/devices/" + device.ID + "/stream')" }
>
```

### Button Actions with `@post` / `@delete`

```templ
// POST action
{ templ.Attributes{"data-on:click": "@post('/ui/devices/" + deviceID + "/services/" + name + "/restart')"}... }

// DELETE with confirmation guard
data-on:click={ "if(!confirm('Delete this alias?')) return; @delete('/ui/devices/" + deviceID + "/firewall/aliases/" + id + "')" }

// GET to load a form/sheet
data-on:click={ "@get('/ui/devices/" + device.ID + "/firewall/aliases/new')" }
```

### OOB (Out-of-Band) Updates

Multiple elements can be patched in a single SSE response by including fragments with different IDs:

```templ
templ DeviceUpdateEvent(d DeviceListView, stats DeviceListStats) {
    <tr id={ "device-row-" + d.ID }>...</tr>
    <div id="device-stats">...</div>
    <span id="navbar-online-count">{ fmt.Sprintf("%d online", stats.Online) }</span>
}
```

### ContentError with Retry

When a NATS request fails, show an error with a retry button:

```templ
templ ContentError(dataURL, message string) {
    <div id="page-content">
        <p>{ message }</p>
        <button data-on:click={ "@get('" + dataURL + "')" }>Retry</button>
    </div>
}
```

## Core Attributes

| Attribute            | Purpose                      | Common Usage       | Key Details                                                            |
|----------------------|------------------------------|--------------------|------------------------------------------------------------------------|
| `data-init`          | Run on initialization        | Yes                | `data-init="@get('/ui/devices/stream')"` for SSE stream init           |
| `data-on`            | Event listeners              | Yes                | `data-on:click`, modifiers: `__debounce`, `__throttle`, `__once`, etc. |
| `data-on-intersect`  | Viewport visibility trigger  | Yes (primary)      | `__once` for lazy loading; `__exit`, `__half`, `__full`, `__threshold` |
| `data-signals`       | Declare/patch signals        | Minimal            | `__ifmissing` for defaults; `_` prefix = private                       |
| `data-bind`          | Two-way binding              | —                  | Works with input/select/textarea                                       |
| `data-show`          | Conditional visibility       | —                  | **Must** add `style="display: none"` to prevent flicker                |
| `data-class`         | Conditional classes          | —                  | Object syntax `{'class': $signal}`                                     |
| `data-text`          | Bind text content            | —                  | Auto-updates on signal change                                          |
| `data-attr`          | Bind any attribute           | —                  | Object syntax or named `data-attr:name="expr"`                         |
| `data-computed`      | Derived signals              | —                  | Read-only; no side effects                                             |
| `data-ref`           | DOM element reference        | —                  | Creates signal pointing to element                                     |
| `data-indicator`     | Fetch status tracking        | —                  | Boolean signal; true while fetching                                    |
| `data-effect`        | Side effects                 | —                  | Runs on load + whenever dependencies change                            |
| `data-ignore`        | Skip Datastar processing     | —                  | `__self` modifier to only skip element                                 |
| `data-ignore-morph`  | Skip morphing                | —                  | For elements managed by external libraries                             |
| `data-preserve-attr` | Keep attributes during morph | —                  | Space-separated list of attribute names                                |
| `data-on-interval`   | Timed execution              | —                  | Default 1s; `__duration` modifier to customize                         |
| `data-persist`       | Local/session storage        | —                  | Pro: `__session` modifier; filter with regex                           |
| `data-replace-url`   | URL without reload           | —                  | Expression-evaluated template literal                                  |

## Action Plugins

| Action                   | Purpose                  | Common Usage       | Key Details                           |
|--------------------------|--------------------------|--------------------|---------------------------------------|
| `@get(url, opts)`        | GET request              | Yes                | Signals as query params; SSE response |
| `@post(url, opts)`       | POST request             | Yes                | Signals in JSON body                  |
| `@delete(url, opts)`     | DELETE request           | Yes                | Same pattern as @post                 |
| `@put(url, opts)`        | PUT request              | —                  | Same pattern as @post                 |
| `@patch(url, opts)`      | PATCH request            | —                  | Same pattern as @post                 |
| `@setAll(value, filter)` | Bulk signal update       | —                  | Filter with include/exclude regex     |
| `@toggleAll(filter)`     | Bulk boolean toggle      | —                  | Same filter pattern                   |
| `@peek(fn)`              | Read without subscribing | —                  | Prevents reactivity triggers          |
| `@clipboard(text)`       | Copy to clipboard        | —                  | Pro; optional base64                  |

## Action Options (for @get/@post etc.)

| Option                | Default                                  | Purpose                                 |
|-----------------------|------------------------------------------|-----------------------------------------|
| `contentType`         | `'json'`                                 | `'json'` or `'form'`                    |
| `filterSignals`       | `{include: /.*/, exclude: /(^_\|._).*/}` | Control which signals are sent          |
| `selector`            | `null`                                   | Target specific form                    |
| `headers`             | `{}`                                     | Custom HTTP headers                     |
| `openWhenHidden`      | varies                                   | Keep SSE open when tab hidden           |
| `retry`               | `'auto'`                                 | `'auto'`/`'error'`/`'always'`/`'never'` |
| `retryInterval`       | `1000`                                   | Initial retry delay (ms)                |
| `retryScaler`         | `2`                                      | Exponential backoff multiplier          |
| `retryMaxWaitMs`      | `30000`                                  | Max retry interval                      |
| `retryMaxCount`       | `10`                                     | Max retry attempts                      |
| `requestCancellation` | `'auto'`                                 | `'auto'`/`'disabled'`/AbortController   |

## SSE Event Types

**`datastar-patch-elements`** — Morphs DOM fragments

- Parameters: `selector`, `mode`, `namespace`, `useViewTransition`
- Mode values: `outer` (default), `inner`, `replace`, `prepend`, `append`, `before`, `after`, `remove`
- Default `outer` replaces the matched element including its tag

**`datastar-patch-signals`** — Updates reactive signals

- Parameters: `signals` (JSON object), `onlyIfMissing`
- Set signal to `null` to remove it

## Anti-Flicker Pattern

Elements using `data-show` must include `style="display: none"`:

```templ
// Correct — hidden until Datastar evaluates the expression
<div style="display: none" data-show="$someSignal">

// Wrong — flashes visible before Datastar hides it
<div data-show="$someSignal">
```

## The Tao of Datastar

1. **State in the Right Place** — Backend is the source of truth
2. **Start with the Defaults** — Question before customizing
3. **Patch Elements & Signals** — Backend actively drives frontend
4. **Use Signals Sparingly** — Reserve for user interactions and form binding
5. **In Morph We Trust** — Send large DOM trees; morphing diffs efficiently
6. **SSE Responses** — Stream 0-to-n patches via text/event-stream
7. **Compression** — Compress SSE with Brotli for large DOM chunks
8. **Backend Templating** — Use Templ to stay DRY
9. **Page Navigation** — Use anchors and redirects
10. **Browser History** — Let browsers manage history
11. **CQRS** — Separate reads (long-lived SSE) from writes (short-lived POST)
12. **Loading Indicators** — Use data-indicator for fetch status
13. **No Optimistic Updates** — Never update UI before backend confirms
14. **Accessibility** — Semantic HTML, ARIA attributes, keyboard support