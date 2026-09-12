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

| CLI | Login command (your session) | Default profile |
|---|---|---|
| **Claude Code** | `claude auth status` | `claude` |
| **OpenAI Codex** | `codex login status` | `codex` |
| **Gemini CLI**  | `gemini auth status` | `gemini` |
| **OpenCode**    | `opencode auth list` | `opencode` |

`Local AI Proxy` wraps these CLIs behind a **single OpenAI-compatible HTTP
endpoint** (`127.0.0.1:8317/v1`), so any tool that speaks the OpenAI Chat API
can talk to any of them — **without** giving that tool your CLI credentials.

Requests reference **model profiles**. Each profile has an id (what clients
send as `model`), a backend provider, a streaming mode and a per-model timeout.
The four defaults (`claude`, `codex`, `gemini`, `opencode`) are created on
first run; add as many as you like from the Dashboard.

It runs a desktop control panel (Wails) with five tabs —
Dashboard · Models · Providers · Activity · Settings — that lets you:
- Start/stop/restart the local server and change its port (live, pre-bind tested, with rollback).
- Create/edit/duplicate/delete/test model profiles, pick upstream models, and copy a **Connect** snippet (OpenAI, Cline, Roo Code, Aider, Python SDK, ainovel-cli).
- Enable/disable each provider, tune concurrency & queue, see honest capabilities and live queue depth.
- Watch activity metadata (status, queue wait, duration, TTFT — never prompts).
- Tune the global concurrency safety limit (default 1).
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
3. **Queues, not a free-for-all.** A global safety semaphore (default `1`) plus
   one shared queue per provider (concurrency cap + bounded queue with
   queue/exec timeouts) — so a flood of requests doesn't stack 50 terminals,
   and extra model profiles can never bypass provider limits.
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
- Live server status, an editable port, and a table of model profiles.

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

> Replace `model` with any profile id from the Dashboard — the defaults are
> `claude`, `codex`, `gemini`, `opencode`. Add more profiles (e.g. a
> `fast` model on the gemini backend) and send those ids instead.

---

## 📡 API reference

The HTTP API is OpenAI/Chat-compatible. Base URL: `http://127.0.0.1:8317/v1`.

| Endpoint | Method | Description |
|---|---|---|
| `/v1/chat/completions` | POST | Chat completions (OpenAI format, streaming or not) |
| `/v1/completions` | POST | Alias of chat completions |
| `/v1/responses` | POST | Responses API (same core as chat completions) |
| `/v1/models` | GET | List enabled model profile ids |
| `/health` | GET | Liveness + provider overview |
| `/health?port=...` | GET | Same |

### Streaming

`stream: true` returns OpenAI-style SSE chunks (`data: { ...chat.completion.chunk }`
events with `finish_reason:"stop"`, ended by `data: [DONE]`) using **real token
streaming where the CLI supports it**:

| Backend | Default profile mode | Streaming source |
|---|---|---|
| Claude Code | `native` | `--output-format stream-json` `content_block_delta` events |
| Codex | `disabled` | `codex exec --json` JSON-lines assistant messages |
| Gemini CLI | `disabled` | raw stdout lines |
| OpenCode | `native` | raw stdout lines |

Each profile has a `streamMode` (`native` or `disabled`). A `disabled` profile
answers as one-shot JSON and **refuses** `stream:true` with
`400 streaming_not_supported` rather than faking near-real-time output — set
`stream:false` (or omit it) for those.

A failed stream emits an SSE `error` event with `finish_reason:"error"`
(never `stop`), then `data: [DONE]` — so failures, cancellations and timeouts
are never recorded as success.

`stream_options: {"include_usage": true}` forwards real token counts when the
CLI reports them; counts are never fabricated.

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
  }]
}
```

> `usage` is only present when the CLI truly reports token counts — never fabricated.

### Tools

CLI backends don't support OpenAI tool calling, so requests with `tools` /
`tool_choice` fail fast with `400 unsupported_feature` instead of being
silently ignored. Rich message content (text parts), `response_format` and
`stream_options` are accepted and mapped onto the shared core.

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

| Alias | CLI binary | Invocation (one-shot) | Output |
|---|---|---|---|
| `claude` | `claude` | `claude -p <prompt> --output-format json --permission-mode plan [--model M]` | JSON `result` field |
| `codex` | `codex` | `codex exec --json --ephemeral --sandbox read-only --skip-git-repo-check -` (prompt via stdin, unique `--output-last-message` temp file per request) | last-message file, else JSONL assistant message |
| `gemini` | `gemini` | `gemini -p <prompt> [--model M]` | raw stdout text |
| `opencode` | `opencode` | `opencode run --format json [-m M] <prompt>` | JSON text/result/content, else raw stdout |

Streaming: `claude` uses `--output-format stream-json` (`content_block_delta`);
codex parses JSONL assistant messages; gemini/opencode forward stdout lines.
All backends advertise honest `Capabilities` (streaming, model selection, …) —
the UI never offers native streaming where the provider lacks it.

> Discovery uses **each CLI's own auth/status command** (`codex login status`,
> `claude auth status`, ...) — it never guesses and never spends quota probing.

---

## 🗂 Architecture

```
                   ┌────────────────────────────────────────────┐
                   │  frontend/  (Wails v2, vanilla HTML/CSS/JS) │
                   │  Dashboard · Models · Providers ·           │
                   │  Activity · Settings + Connect helper       │
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
                   │  global semaphore (default 1) +             │
                   │  one shared queue per PROVIDER              │
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

### Scheduling (the important part)

Concurrency lives at **two levels**, never per-model:

1. **Global safety semaphore** (default `1`) — only that many CLI executions
   run at once across *all* providers. Extra requests queue, never drop.
2. **One shared queue per provider** — every model profile on `claude`
   (e.g. `architect`, `claude-review`) shares the same Claude scheduler, so
   new profiles can never bypass provider concurrency.

### Data flow (run a request)

1. `api.Server.handleChat` validates auth + parses the OpenAI request into the
   canonical `provider.Request` (tools, response_format, content parts mapped).
2. `core.RunChat` resolves the model profile → provider, enriches upstream
   model/system prompt/limits, takes a global slot, submits to that
   **provider's** queue.
3. The queue respects `concurrency` + `maxQueue`; the runner
   (`internal/proc.Runner`) spawns the CLI with `exec.CommandContext`.
4. The adapter's `Parse` reads stdout/stderr into a `provider.Result`.
5. `api` maps errors to HTTP status codes (401/429/502/504 ...), writes the
   OpenAI-shaped JSON/SSE response, and records sanitized activity metadata
   (status, queue wait, duration, TTFT — never prompts).

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
run, atomic write). Fields in the app's Settings tab map directly to it. Files
from earlier versions are migrated automatically.

```json
{
  "version": 3,
  "server": { "host": "127.0.0.1", "port": 8317 },
  "autoStartServer": true,
  "requireApiKey": false,
  "apiKey": "",
  "globalConcurrency": 1,
  "providers": {
    "claude":  { "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 },
    "codex":   { "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 },
    "gemini":  { "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 },
    "opencode":{ "enabled": true,  "concurrency": 1, "maxQueue": 10, "queueTimeoutSec": 120, "execTimeoutSec": 60 }
  },
  "models": {
    "claude":   { "provider": "claude",   "stream_mode": "native",   "timeout_seconds": 300, "enabled": true },
    "codex":    { "provider": "codex",    "stream_mode": "disabled", "timeout_seconds": 300, "enabled": true },
    "gemini":   { "provider": "gemini",   "stream_mode": "disabled", "timeout_seconds": 300, "enabled": true },
    "opencode": { "provider": "opencode", "stream_mode": "native",   "timeout_seconds": 300, "enabled": true }
  },
  "saveLogsToDisk": false,
  "retentionDays": 7,
  "debugLogging": false
}
```

Model profiles also accept `upstream_model`, `temperature`, `max_tokens`,
`context_window`, `system_prompt`, `extra_args`, `working_dir` and other
future policy fields — persisted with `omitempty` so old files migrate
without loss. Non-loopback hosts require explicit
`server.allowNonLoopback` opt-in, otherwise the host resets to `127.0.0.1`.

**OAuth credentials are never stored by this app.** Each CLI owns its own
login/session; the proxy just calls the CLI as the current user.

---

## 🔒 Security & privacy

- **Loopback only.** The HTTP server binds `127.0.0.1` — it is never exposed on
  your LAN/internet. Non-loopback hosts need an explicit
  `allowNonLoopback` opt-in. Breaking the `127.0.0.1` guarantee is a bug.
- **No permissive CORS.** No `Access-Control-Allow-Origin: *` — local
  CLI/desktop clients don't need it.
- **No telemetry.** Nothing phones home. No crash reports, no analytics, no CDN.
- **No credentials stored.** The app never sees your provider login. CLI auth
  stays in each CLI's own config.
- **Input is never logged.** Prompts, messages, API keys and bearer tokens are
  not written to disk or the ring buffer — only sanitized *metadata* (time,
  model, provider, status, queue wait, duration, TTFT) is.
- **Optional API key.** When enabled, every `/v1` call must send
  `Authorization: Bearer <key>` (exact, case-sensitive, constant-time comparison).
- **No aggressive retry.** Auth/quota/cancelled errors are never retried automatically.

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
│   └── queue/             # per-model concurrency + bounded queue
├── frontend/dist/         # static UI (HTML/CSS/JS, embedded)
└── docs/                  # architecture + CLI discovery notes
```

### OpenAPI surface notes

- `model` must be an enabled model profile id (defaults: `claude`, `codex`,
  `gemini`, `opencode`), else `400 model_not_found`.
- `stream: true` on a profile with `streamMode: disabled` →
  `400 streaming_not_supported` (never a silent conversion to JSON).
- Empty `messages` → `400 invalid_request_error`.
- Disabled profile → `400 provider_disabled`; backend not installed → `503 provider_unavailable`.

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
