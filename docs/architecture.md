# Architecture

```text
        Wails shell (WebView2)
             | window.go.main.App.* calls
             v
        app.go  — thin Wails binding layer (no logic)
             |
             v
      internal/core  Core        — config + discovery cache + queues + activity + HTTP server
             |  implements Backend interface        (avoids import cycle with api)
             v
      internal/api  Server      — net/http handler, routing, auth middleware, error mapping
             |
             | provider.Adapter (claude.codex.gemini.opencode)
             v
      internal/provider        — per-provider subprocess invocation + message serialization
             | Runner (mockable in tests)
             v
      internal/proc            — subprocess lifecycle, limits, sanitization, Windows tree-kill
```

Support packages:

- `internal/config` — typed defaults + atomic JSON load/save at `%APPDATA%\LocalAIProxy\config.json`. Corrupt/missing file ⇒ defaults; saving is write-temp + rename.
- `internal/discovery` — finds executables, reports version + **login status (no quota probing)**, caches results with TTL.
- `internal/queue` — per-provider queue: one gate slot (single-flight per provider) + bounded queue + started-at exec ACL. Busy → 429 `provider_busy`; queue full → 429 `queue_full`; queued too long → 503 `queue_timeout`; exec timeout → 504 `gateway_timeout`.
- `internal/activity` — RAM ring buffer of API requests for the UI (no disk).
- `internal/logr` — optional sanitized file logs under `%APPDATA%\LocalAIProxy\logs\` with retention pruning.
- `internal/api` also serves `/health` (self-check; never probes model quota).

## HTTP surface (all bound to 127.0.0.1 only)

| Route | Method | Notes |
|---|---|---|
| `/health` | GET | `{"status":"ok","providers":4}` |
| `/v1/models` | GET | always returns all 4 aliases incl. disabled/uninstalled |
| `/v1/chat/completions` | POST | `model=<alias>`, non-streaming |
| `/v1/completions` | POST | same |
| `/` | GET | CORS preflight / info JSON |

Auth: when API-key mode is on, `Authorization: Bearer <key>` is required (constant-time compare); binding config also honors a configured `apiKey`.

## Request lifecycle

1. HTTP request → auth middleware → parse/validate message.
2. JSON → provider message → `Queue.Submit` with ctx deadline = queue timeout (if the exec timeout is reached, the process is tree-killed and a `gateway_timeout` is returned).
3. Stream request (`"stream":true`) → `400 {"error":{"message":"streaming_not_supported", ...}}`.
4. Provider result → map to chat response; record activity; return.

## UI contract

`Core.Snapshot()` returns everything the UI renders in one call (`serverRunning`, `port`, `url`, `providers[]`, `activity[]`). A `state` event is emitted on any change; the frontend just re-fetches and re-renders. `ProviderView.status/statusKind` is computed by `deriveStatus` — never color-only, always label + kind.

## Error contract (normalized)

`provider.Error` has a fixed `code` set. The API layer maps them to HTTP statuses:

| provider.Error code | HTTP |
|---|---|
| `not_installed` / `not_authenticated` | 503 |
| `provider_disabled` | 503 |
| `invalid_message` | 400 |
| `busy` / `queue_full` | 429 |
| `queue_timeout` | 503 |
| `timeout` / `provider_crashed` / `invalid_output` | 504 |
| unknown | 502 |