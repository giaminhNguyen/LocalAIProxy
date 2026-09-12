# Kiến trúc LocalAIProxy

Tài liệu này mô tả đầy đủ luồng request, từng module và contract giữa chúng, cho con người hoặc agent khác đọc & sửa code mà không cần đọc lại toàn bộ source.

## Mục lục

- [Triết lý thiết kế](#triết-lý-thiết-kế)
- [Sơ đồ module](#sơ-đồ-module)
- [Luồng xử lý request](#luồng-xử-lý-request)
- [HTTP surface](#http-surface)
- [Activity ring buffer](#activity-ring-buffer)
- [Queue & concurrency](#queue--concurrency)
- [Discovery (probe CLI)](#discovery-probe-cli)
- [Process safety (Job Object)](#process-safety-job-object)
- [Logging & sanitize](#logging--sanitize)
- [Config persistence](#config-persistence)
- [Lược đồ data](#lược-đồ-data)
- [Quy ước lỗi](#quy-ước-lỗi)
- [Frontend contract](#frontend-contract)

---

## Triết lý thiết kế

1. **Một CLI = một provider.** Không có "polyfill"/"aggregator" AI: mỗi alias (`claude`, `codex`, `gemini`, `opencode`) gọi **đúng CLI** mà ta đã cài trên máy, qua chínhOAuth/login có sẵn của CLI đó. LocalAIProxy **không bao giờ** quản lý credential.
2. **Giao diện nhỏ, không chromium nặng.** Frontend là 3 file tĩnh (HTML/CSS/JS thuần) nhúng qua `go:embed` — không npm, không build step, không framework.
3. **An toàn mặc định.** Server chỉ bind `127.0.0.1`; CORS mở nhưng chỉ phục vụ local; API key tùy chọn; **không có gì gửi lên mạng**. Không telemetry.
4. **Không bao giờ để orphan process.** Mọi CLI chạy trong **Windows Job Object** với `KILL_ON_JOB_CLOSE`; khi app thoát (kể cả crash/force-kill), OS giết cả cây process.
5. **Config chịu lỗi.** File config hỏng/thiếu → tự về defaults, app không bao giờ crash vì config.

---

## Sơ đồ module

```text
                 ┌──────────────────────────────────────────────┐
                 │              frontend/ (dist)                │
                 │   index.html · css/app.css · js/app.js       │
                 │   (Vanilla JS, Wails runtime bindings)       │
                 └───────────────▲──────────────────────────────┘
                                 │ Wails EventsEmit("state") / bind(App)
                 ┌───────────────┴──────────────────────────────┐
                 │                  app.go (Wails App)           │
                 │  GetSnapshot · RunChat · SetPort · Settings   │
                 │  beforeClose (permissive) · boot()            │
                 └───────────────▲──────────────────────────────┘
                                 │ implements api.Backend
                 ┌───────────────┴──────────────────────────────┐
                 │              internal/core (Core)             │
                 │  discovery cache · queue per provider         │
                 │  activity ring · server lifecycle             │
                 └───────────────▲──────────────┬────────────────┘
                                 │              │
                    api.Backend  │              │ provider.Runner
                 ┌───────────────┴──────┐  ┌─────▼──────────────────┐
                 │    internal/api      │  │    internal/provider    │
                 │  HTTP server (http) │  │  Claude·Codex·Gemini·   │
                 │  /v1/chat/completions│  │  OpenCode adapters      │
                 │  /v1/models /health │  │  parse stdout→Result    │
                 └─────────────────────┘  └─────┬───────────────────┘
                                                │ Invocation (exec args)
                                 ┌──────────────▼─────────────────┐
                                 │         internal/queue          │
                                 │  gate (maxQueue) + slots (conc)│
                                 └──────────────┬─────────────────┘
                                                │
                                 ┌──────────────▼─────────────────┐
                                 │        internal/proc            │
                                 │  Runner → exec CLI · tree-kill  │
                                 │  Windows Job Object join        │
                                 └────────────────────────────────┘
```

Support packages (không có trong sơ đồ trên): `internal/discovery` (probe CLI), `internal/config` (persistence), `internal/activity` (ring log), `internal/logr` (disk logging). Xem [Lược đồ data](#lược-đồ-data).

---

## Luồng xử lý request

```text
Client (127.0.0.1 only)
   │  POST /v1/chat/completions  {model: "codex", messages:[...]}
   ▼
internal/api.Server
   │  withCORS → withAuth (optional Bearer key)
   │  validate: method POST, model ∈ aliases, messages ≥ 1, stream != true
   ▼
api.handleChat
   │  parse JSON body → provider.Request{Model, Messages}
   ▼
Core.RunChat(ctx, req)
   │  provider alias = req.Model
   │  check provider enabled + installed + auth
   │  Queue.Submit(ctx)  ← bounded queue (gate) + concurrency (slots)
   ▼
queue/submit → proc.Runner.Run(ctx, Invocation)
   │  exec.CommandContext(claude|codex|gemini|opencode, args)
   │  join Windows Job Object (reap tree on close)
   │  cap stdout/stderr (8 MiB/stream)
   ▼
provider adapter Parse(stdout, stderr) → provider.Result{Content}
   └ (on error) → provider.Error{Code, Status, Message, Details(sanitized)}
   ▼
api maps error → HTTP status → {error:{type,message,status}}
```

Chi tiết mã: `internal/api/api.go` (handlers), `internal/core/core.go` (RunChat + queues), `internal/queue/queue.go` (submit), `internal/proc/proc.go` (runner + tree killer).

---

## HTTP surface

`internal/api` cung cấp:

| Route | Method | Mô tả |
|---|---|---|
| `/health` | GET | `{"status": "ok"|"stopped", "server": {...}}` — không probe quota |
| `/v1/models` | GET | Danh sách 4 alias (luôn trả tất cả, kể cả disabled/not-installed) |
| `/v1/chat/completions` | POST | Chat completions thật |

Middleware: `withCORS` (mở, nhưng server chỉ nghe 127.0.0.1), `withAuth` (only nếu provider config yêu cầu API key).

Streaming: không hỗ trợ — `"stream": true` → `400 streaming_not_supported`. Trả đúng OpenAI `chat.completion` format (id bắt đầu `chatcmpl-`).

---

## Activity ring buffer

`internal/activity` — in-memory, không ghi đĩa trừ khi người dùng bật "Save logs".

```text
type Log struct { mu; items []Entry; max int }   // max thường = 50
Entry{ Time, Provider, Alias, Status, OK, DurationMS, Message }
```

- `Add` prepend (mới nhất trước), giữ tối đa `max` bản ghi.
- `List` trả copy (lock), UI render không đụng bộ đệm nội bộ.
- **Không bao giờ log** prompt/auth header/API key/token.

---

## Queue & concurrency

`internal/queue` — mỗi provider một queue riêng, định nghĩa bởi `ProcConfig`:

- `gate`: buffered channel `maxQueue` — cấp "ticket" khi vào hàng đợi. Hết chỗ → `429 provider_busy`.
- `slots`: buffered channel `concurrency` — cấp quyền chạy CLI thật. Đợi slot tiêu tốn queue timeout (`queueTimeoutSec`).
- Timeout exec (`execTimeoutSec`) áp quanh `Run` — thường 0 = unlimited.
- Visitor không có "wait queue": nếu hết slot + hết chỗ trong gate thì từ chối ngay thay vì kẹt.

Xem test `internal/queue/queue_test.go` để hiểu hành vi cạnh biên (timeout, cancel, full).

---

## Discovery (probe CLI)

`internal/discovery` — tìm + probe các CLI đã cài:

- `Aliases`/`ExecNames`: `claude` → `claude`, `codex` → `codex`, `gemini` → `gemini` (+ `antigravity`), `opencode` → `opencode`.
- `FindExecutable`: `exec.LookPath`; fallback cho `opencode` ở `%USERPROFILE%\.opencode\bin`.
- `Probe` chạy song song (mỗi alias 15s timeout), trả `Info{Alias, Name, Executable, Installed, Version, Auth}`.
- Auth detect: chạy lệnh status riêng của từng CLI (`claude auth status`, `codex login status`, `gemini auth status`, `opencode auth list`), parse JSON/text, không gọi AI.

---

## Process safety (Job Object)

`internal/proc` + `proc_windows.go`:

- Mỗi req spawn CLI qua `exec.CommandContext`.
- **Windows Job Object** được tạo một lần (`jobOnce`) với `KILL_ON_JOB_CLOSE`. `joinJob(pid)` gắn process con vào job.
- Khi app đóng/crash/force-kill: OS tự giết toàn bộ cây process thuộc job → không có orphan CLI.
- `treeKiller` (proc.go) là backstop: khi ctx bị cancel, `taskkill /PID <pid> /T /F` giết cả nhánh.
- Output cap 8 MiB (`maxCapture`), buffer giới hạn `limitedBuffer`.

`proc_windows.go` dùng syscall trực tiếp (`windows.CreateJobObject`, `SetInformationJobObject`) — không cần thêm dependency.

---

## Logging & sanitize

`internal/logr` (disk logging optional), `internal/activity` (ring in-memory), `internal/logr`:

- `logr` ghi file sanitized trong `%APPDATA%\LocalAIProxy\logs\`, có retention+prune (`RetentionDays`), cắt path home (`<user>`), debug flag.
- `sanitize` (trong proc.go): thay home dir bằng `<user>`, truncate 4KB, chỉ giữ lại message thân thiện; không đưa raw stderr lên UI/API.

---

## Config persistence

`internal/config`:

- Vị trí: `%APPDATA%\LocalAIProxy\config.json` (Windows) — dùng `os.UserConfigDir()`.
- `Config{ Port, AutoStartServer, RequireAPIKey, APIKey, Providers{...}, RetentionDays, DebugLogging }`.
- Mỗi `ProviderConfig`: `Concurrency`, `MaxQueue`, `QueueTimeoutSec`, `ExecTimeoutSec` (mặc định Concurrency=1, MaxQueue=10, QueueTimeout=120, ExecTimeout=0).
- `LoadFile` fallback defaults khi thiếu/hỏng; `Save` ghi atomic (tmp+rename).

---

## Lược đồ data

Các kiểu chủ chốt:

```text
provider.Request   { Model string;  Messages []Message }
provider.Message   { Role Role(system|user|assistant);  Content string }
provider.Invocation{ Exec string; Args []string; Stdin string; Env []string }
provider.Result    { Content string }
provider.Error     { Code string; Provider string; Message string; Status int; Details string }
provider.ProviderInfo{Known aliases, enabled/disabled, concurrency...}
```

`api`/`core`/`provider`/`queue`/`proc` — mọi boundary dùng `provider.Request/Result/Error` (định nghĩa chung), không leak kểu HTTP vào nội bộ.

---

## Quy ước lỗi

`provider.Error.Code` là chuỗi ổn định; `api` ánh xạ sang HTTP status theo bảng trong README.

| provider code | HTTP |
|---|---|
| `invalid_request_error` / `model_not_found` / `provider_disabled` / `streaming_not_supported` | 400 |
| `provider_authentication_error` | 401 |
| `provider_rate_limited` / `provider_busy` | 429 |
| `provider_timeout` / `provider_process_timeout` | 504 |
| `provider_unavailable` / `queue_timeout` | 503 |
| `provider_process_error` (generic) | 502 |

---

## Frontend contract

- `GetSnapshot()` trả `Snapshot{Server, Providers[], Activity[]}`; UI render mỗi khi nhận `state` event.
- `Snapshot.Providers[i]`: `Alias, Name, Enabled, Installed, Version, Executable, Auth, Status, StatusKind, Concurrency...`.
- Các action binding trong `app.go`: `RunChat`, `StartServer/StopServer/RestartServer`, `SetPort`, `SetAPIKeyEnabled`, `Configure`, `TestProvider`, `SaveLogging`.
- Xem `docs/ui-design-guide.md` cho quy ước CSS/layout.

---

Tài liệu liên quan: [README](../README.md) · [cli-discovery.md](cli-discovery.md) · [ui-design-guide.md](ui-design-guide.md).
