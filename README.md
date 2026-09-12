<div align="center">

# 🏠 Local AI Proxy

**A tiny local HTTP proxy that turns the AI CLIs you already have installed — Claude Code, OpenAI Codex, Gemini CLI, OpenCode — into one OpenAI-compatible API on `127.0.0.1`.**

Everything runs **locally**. No cloud, no accounts, no telemetry, no credentials stored.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Wails](https://img.shields.io/badge/Wails-v2.13-2E7CF6?style=flat-square&logo=wails)](https://wails.io)
[![Windows](https://img.shields.io/badge/Windows-10%2F11-0078D6?style=flat-square&logo=windows&logoColor=white)](https://github.com/)
[![License](https://img.shields.io/badge/License-MIT-28A745?style=flat-square)](LICENSE)
[![Status](https://img.shields.io/badge/status-active-28A745?style=flat-square)](#)

</div>

---

## 🧭 Table of contents

- [What it does](#what-it-does)
- [Why it exists](#why-it-exists)
- [Quick start](#quick-start)
- [API reference](#api-reference)
- [Providers](#providers)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Security & privacy](#security--privacy)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## 🎯 What it does

You already have powerful **native AI CLIs** on your Windows machine, each with its
own login/session, quota and supported model set:

| CLI | Login command (your session) | Model alias |
|---|---|---|
| **Claude Code** | `claude auth status` | `claude` |
| **OpenAI Codex** | `codex login status` | `codex` |
| **Gemini CLI**  | `gemini auth status` | `gemini` |
| **OpenCode**    | `opencode auth list` | `opencode` |

`Local AI Proxy` wraps these CLIs behind a **single OpenAI-compatible HTTP
endpoint** (`127.0.0.1:8317/v1`), so any tool that speaks the OpenAI Chat API
can talk to any of them — **without** giving that tool your CLI credentials.

It runs a desktop control panel (Wails) that lets you:
- Start/stop/restart the local server and change its port.
- Enable/disable each provider, tune concurrency & queue.
- Test each provider on demand.
- Watch live activity history.
- Optionally require an API key for local calls.
- Optionally save sanitized logs to disk with retention policy.

---

## 🤔 Why it exists

**Problem:** The new generation of AI coding agents all ship *different* CLIs
with *different* auth flows. Tools that only know how to call `claude`
can't reach `codex`; a local GUI like Jan or Continue.dev has nowhere to point
for a model that lives in a CLI.

**Ghost processes:** naïvely shelling out to a CLI and killing it on stop
leaves **orphaned children** (and child trees) running forever on Windows.

**Local AI Proxy's answer:**

1. **Probe, don't hardcode.** At startup it discovers which CLIs are installed,
   reads their version and their *login* status in parallel (never spends quota).
2. **One process per request, reaped.** Every CLI spawns as a subprocess with a
   proper context, bounded output capture, and **Job-Object tree-kill** so
   closing the app never leaves orphan AI processes.
3. **A queue, not a free-for-all.** Per-provider concurrency cap + bounded queue
   with queue/exec timeouts — so a flood of requests doesn't stack 50 terminals.
4. **Sanitized everything.** Home dir paths are scrubbed, output is truncated,
   and prompts/keys/tokens are never logged.

---

## 🚀 Quick start

```bash
# build the desktop app
wails build

# run it
./build/bin/LocalAIProxy.exe
```

The app starts its local server automatically. The dashboard shows:
- API URL: `http://127.0.0.1:8317/v1`
- Provider status (installed / auth / ready) per alias.

### Point a tool at it

```bash
curl -X POST http://127.0.0.1:8317/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-api-key-if-enabled>" \
  -d '{
    "model": "claude",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

> Replace `model` with any alias from the Providers tab: `claude`, `codex`,
> `gemini`, `opencode`.

---

## 📡 API reference

The HTTP API is OpenAI/Chat-compatible. Base URL: `http://127.0.0.1:8317/v1`.

| Endpoint | Method | Description |
|---|---|---|
| `/v1/chat/completions` | POST | Chat completions (OpenAI format, non-streaming) |
| `/v1/completions` | POST | Alias of chat completions |
| `/v1/models` | GET | List available model aliases |
| `/health` | GET | Liveness + provider overview |
| `/health?port=...` | GET | Same |

### Request / response

```jsonc
// POST /v1/chat/completions
{
  "model": "codex",              // provider alias
  "messages": [
    { "role": "system", "content": "You are a helpful assistant." },
    { "role": "user", "content": "Write a haiku about Go." }
  ]
}
```

```json
{
  "id": "chatcmpl-1720000123456",
  "object": "chat.completion",
  "model": "codex",
  "choices": [{
    "index": 0,
    "message": { "role": "assistant", "content": "Concurrency locked...\nA goroutine awakens,\nNil is never nil." },
    "finish_reason": "stop"
  }],
  "usage": { "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0 }
}
```

### Errors

Errors are returned as OpenAI-style `{ "error": { message, type } }` with a
provider-aware `type` and a sanitized `message`. Example:

```json
{ "error": { "message": "Claude requires authentication. Run: claude auth status", "type": "provider_authentication_error" } }
```

---

## 🔌 Providers

Each CLI adapter lives in `internal/provider`. The runner (`internal/proc`)
turns an `Invocation` into a real process and back into a `Result`.

| Alias | CLI binary | Invocation | Parse |
|---|---|---|---|
| `claude` | `claude` | `claude -p <prompt> --output-format json --permission-mode plan` | JSON `result` field |
| `codex` | `codex` | `codex exec --json -c loader.py ...` | JSON `result` field |
| `gemini` | `gemini` | `gemini -p <prompt> json` | JSON `result` field |
| `opencode` | `opencode` | `opencode run --format json -m ...` | JSON `result` field |

> Discovery uses **each CLI's own auth/status command** (`codex login status`,
> `claude auth status`, ...) — it never guesses and never spends quota probing.

---

## 🗂 Architecture

```
                   ┌────────────────────────────────────────────┐
                   │  frontend/  (Wails v2, vanilla HTML/CSS/JS) │
                   │  Dashboard · Providers · Settings · Activity│
                   └──────────────────▲─────────────────────────┘
                                      │  wails: Go ↔ JS bindings
                   ┌──────────────────┴─────────────────────────┐
                   │              app.go (App)                   │
                   │  boot · RunChat · Configure · Snapshot ·    │
                   │  Start/Stop/SetPort · TestProvider ·        │
                   └──────────────────▲─────────────────────────┘
                                      │
                   ┌──────────────────┴─────────────────────────┐
                   │           internal/core (Core)              │
                   │  cfg · providers · queues · activity ·      │
                   │  RunChat(ctx, Request) → Result             │
                   └──────────────────▲─────────────────────────┘
                                      │
        ┌──────────────┬──────────────┴──────────────┬──────────────┐
        ▼              ▼                             ▼              ▼
 ┌────────────┐ ┌────────────┐              ┌────────────┐  ┌────────────┐
 │internal/api│ │internal/pv │              │ internal/  │  │ internal/  │
 │ HTTP server│ │ config     │              │  provider  │  │  discovery │
 │ API backend│ │ persistence│              │ CLI adapts │  │ probe/auth │
 └────────────┘ └────────────┘              └────────────┘  └────────────┘
        │                                     │
        ▼                                     ▼
 ┌────────────┐                        ┌─────────────┐
 │ internal/  │                        │ internal/   │
 │  activity  │                        │  queue      │
 │ ring log   │                        │ concurrency │
 └────────────┘                        └──────▲──────┘
                                             │ submit
                                      ┌──────┴──────┐
                                      │ internal/proc│
                                      │ spawn + kill │
                                      │ Job Object   │
                                      └─────────────┘
```

### Data flow (run a request)

1. `api.Server.handleChat` validates auth + parses the OpenAI request.
2. `core.RunChat` resolves the provider, checks installed + auth, builds an
   `Invocation`, submits to that provider's queue.
3. The queue respects `concurrency` + `maxQueue`; the runner
   (`internal/proc.Runner`) spawns the CLI with `exec.CommandContext`.
4. The adapter's `Parse` reads stdout/stderr into a `provider.Result`.
5. `api` maps errors to HTTP status codes (401/429/502/504 ...), writes the
   OpenAI-shaped JSON response, and records activity.

### Process safety (the important part)

Every request runs a **separate CLI process** under
`exec.CommandContext`. When the request's context is cancelled (client
disconnect, timeout, server stop), `internal/proc` kills the process:

- On Windows, the CLI runs inside a **Job Object with `KILL_ON_JOB_CLOSE`**,
  so on app close the OS terminates the **entire process tree** — no orphans.
- `killTree` is a token-based fallback (taskkill /T /F style), invoked only
  while the context-owned watcher is alive, never after exit.
- Output is captured through a **bounded, sanitized buffer** (max 8 MiB) so a
  runaway CLI can't balloon memory or leak home-dir paths into responses.

---

## ⚙️ Configuration

Config is persisted to `%APPDATA%\LocalAIProxy\config.json` (created on first
run, atomic write). Fields in the app's Settings tab map directly to it.

```json
{
  "port": 8317,
  "autoStart": true,
  "requireApiKey": false,
  "apiKey": "",
  "providers": {
    "claude":  { "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 },
    "codex":   { "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 },
    "gemini":  { "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 },
    "opencode":{ "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 }
  },
  "saveLogsToDisk": false,
  "retentionDays": 7,
  "debugLogging": false
}
```

**OAuth credentials are never stored by this app.** Each CLI owns its own
login/session; the proxy just calls the CLI as the current user.

---

## 🔒 Security & privacy

- **Loopback only.** The HTTP server binds `127.0.0.1` — it is never exposed on
  your LAN/internet. Breaking the `127.0.0.1` guarantee is a bug.
- **No telemetry.** Nothing phones home. No crash reports, no analytics, no CDN.
- **No credentials stored.** The app never sees your provider login. CLI auth
  stays in each CLI's own config.
- **Input is never logged.** Prompts, messages, API keys and bearer tokens are
  not written to disk or the ring buffer — only sanitized *metadata* (time,
  provider, duration, status) is.
- **Optional API key.** When enabled, every `/v1` call must send
  `Authorization: Bearer <key>` (constant-time comparison).

---

## 🛠 Development

```bash
go mod tidy          # fetch deps (Wails v2, golang.org/x/sys)
go build ./...       # compile all packages
go test ./...        # run unit tests
go vet ./...         # static analysis

wails build          # build the full desktop exe (embeds frontend)
wails dev            # live dev with reload & browser tools
```

### Layout

```
LocalAIProxy/
├── app.go                 # Wails App: bindings, boot, lifecycle
├── main.go                # Wails entrypoint, embeds frontend/dist
├── internal/
│   ├── api/               # HTTP API server (OpenAI-compatible)
│   ├── activity/          # ring-buffer activity log
│   ├── config/            # typed config + persistence
│   ├── core/              # app logic: run, queues, snapshot
│   ├── discovery/         # CLI probe + auth detection
│   ├── proc/              # spawn/kill, Job Object, sanitize  ← Windows tree-kill
│   ├── provider/          # request/result types + per-CLI adapters
│   └── queue/             # per-provider concurrency + bounded queue
├── frontend/dist/         # static UI (HTML/CSS/JS, embedded)
└── docs/                  # architecture + CLI discovery notes
```

### OpenAPI surface notes

- `model` must be one of the 4 aliases, else `400 model_not_found`.
- `stream: true` is currently **not supported** → `400 streaming_not_supported`.
- Empty `messages` → `400 invalid_request_error`.
- Provider disabled → `403 provider_disabled`; not installed → `400 model_not_found`.

---

## 🤝 Contributing

This is a small, deliberately boring codebase. Before adding features:

1. **Prefer the standard library.** The only external deps are Wails + x/sys.
2. **Keep it local-first.** Anything that touches the network beyond
   127.0.0.1 is almost certainly wrong.
3. **Keep it lazy.** Reuse `internal/proc`, `internal/queue`, `internal/config`
   instead of duplicating logic.
4. Run `go vet ./...` and `go test ./...` before pushing.

---

## 📄 License

[MIT](LICENSE) © Local AI Proxy contributors

---

*Reads only what your machine already knows. Talks only to itself.*
