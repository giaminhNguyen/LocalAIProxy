# Local AI Proxy

A Windows control panel that exposes your already-installed AI CLIs (Claude Code, OpenAI Codex, Gemini CLI, OpenCode) behind **one local OpenAI-compatible HTTP API** on `127.0.0.1`. It uses each CLI's existing login — you never hand Local AI Proxy a credential.

- No telemetry. Everything stays on `127.0.0.1`.
- Stateless: one subprocess per request, no conversation memory.
- Non-streaming MVP: `stream:true` requests are rejected with `streaming_not_supported`.

## Supported providers

| Alias | CLI binary | Login check |
|---|---|---|
| `claude` | Claude Code    | `claude auth status` |
| `codex` | Codex          | `codex login status` |
| `gemini`| Gemini CLI     | `gemini auth status` |
| `opencode` | OpenCode  | `opencode auth list` |

See `docs/cli-discovery.md` for exact invocation flags and accepted output formats.

## Use

1. Build: `wails build` (needs Wails v2; `C:\Users\ming\go\bin\wails.exe`). Output: `build/bin/LocalAIProxy.exe`.
2. Launch the app. Press the **Run server** button (autostart also available in Settings).
3. Point whatever speaks OpenAI chat-completions at:

```
POST http://127.0.0.1:8317/v1/chat/completions
Content-Type: application/json
Authorization: Bearer <key-if-enabled>

{"model": "claude", "messages": [{"role": "user", "content": "hi"}]}
```

`/v1/models` lists all four aliases (even if a CLI isn't installed — the request will fail then). No HTTPS, no proxy fallback, loopback only by design.

## Config & data

- Settings: `%APPDATA%\LocalAIProxy\config.json` (safe atomic writes; corrupt file ⇒ defaults).
- Optional logs: `%APPDATA%\LocalAIProxy\logs\` with retention pruning. The GUI records request history in memory only.

## Docs

- `docs/architecture.md` — modules, HTTP contract, error mapping.
- `docs/cli-discovery.md` — per-CLI flags, login checks, parsing, machine notes.
- `docs/ui-design-guide.md` — UI rules for anyone touching `frontend/dist`.

## Development

```
go build ./...        # backend
go test ./...         # unit tests (see internal/*/ *_test.go)
wails build           # embed frontend/dist and produce the exe
```

Frontend is three hand-written files in `frontend/dist/` embedded with `go:embed` — no npm/Vite step.