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

1. **Một CLI = một provider, một profile = một route.** Provider (`claude`, `codex`, `gemini`, `opencode`) gọi **đúng CLI** đã cài trên máy, qua chính OAuth/login có sẵn của CLI đó. **Model profile** là lớp riêng: client gửi `model=<profile id>`, core resolve ra provider + stream mode + timeout. LocalAIProxy **không bao giờ** quản lý credential. Không fallback ngầm giữa các provider.
2. **Giao diện nhỏ, không chromium nặng.** Frontend là HTML/CSS/JS thuần nhúng qua `go:embed` — không npm, không build step, không framework (CSS transition thuần, không GSAP).
3. **An toàn mặc định.** Server chỉ bind `127.0.0.1` (cấu hình được trong `server.host`); CORS mở nhưng chỉ phục vụ local; API key tùy chọn; **không có gì gửi lên mạng**. Không telemetry.
4. **Không bao giờ để orphan process.** Mọi CLI chạy trong **Windows Job Object** với `KILL_ON_JOB_CLOSE`; khi app thoát (kể cả crash/force-kill), OS giết cả cây process.
5. **Config chịu lỗi + migrate.** File config hỏng/thiếu → tự về defaults; file v1 (port top-level, không `models`) được nâng cấp tự động, app không bao giờ crash vì config.

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
                 │  discovery cache · queue per MODEL id         │
                 │  activity ring · server lifecycle             │
                 │  RunChat / RunChatStream · resolveModel       │
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
   │  POST /v1/chat/completions  {model: "claude-fast", messages:[...]}
   │  hoặc {model, messages, stream:true}
   ▼
internal/api.Server
   │  withCORS → withAuth (optional Bearer key)
   │  handleChat: validate POST, model ∈ enabled profiles, messages ≥ 1
   │  handleChatStream: cùng validate, rồi SSE
   ▼
Core.resolveModel(id)
   │  profile = cfg.Model(id)        → 400 model_not_found nếu thiếu
   │  profile.Enabled                → 400 provider_disabled nếu tắt
   │  resolveProvider → adapter + installed + auth  → 503 provider_unavailable
   ▼
Core.RunChat / RunChatStream
   │  (stream) profile.StreamMode != "native" → 400 streaming_not_supported
   │  adapter.Invoke(req) / adapter.StreamInvoke(req) → provider.Invocation
   │  Invocation{Exec, Args, Stdin, [StreamParse]}
   │  Queue.Submit(ctx) / Queue.SubmitStream(ctx, emit)
   ▼
internal/queue → internal/proc.Runner
   │  RunStream: chạy CLI, đọc stdout từng dòng, emit StreamEvent{Text}
   │  parser dòng: StreamParseLine(line) → (delta, done, err)
   ▼
provider adapter Parse / StreamParse
   │  claude: –output-format json | stream-json (content_block_delta)
   │  codex/gemini/opencode: JSON JSONL / raw stdout
   │  → provider.Result{Content} (stream: cộng dồn delta)
   ▼
api maps error → HTTP status → {error:{type,message,status}}
   (stream: lỗi ghi thành SSE error event rồi data: [DONE])
```

Chi tiết mã: `internal/api/api.go` (handlers), `internal/core/core.go` (RunChat/RunChatStream + resolveModel + queues), `internal/queue/queue.go` (submit/submitStream), `internal/proc/proc.go` (Run + RunStream + tree killer).

---

## HTTP surface

`internal/api` cung cấp:

| Route | Method | Mô tả |
|---|---|---|
| `/health` | GET | `{"status": "ok"|"stopped", "server": {...}}` — không probe quota |
| `/v1/models` | GET | Danh sách **profile đang enabled** (id, không phải alias) |
| `/v1/chat/completions` | POST | Chat completions thật |
| `/v1/completions` | POST | Alias của chat completions |

Middleware: `withCORS` (mở, nhưng server chỉ nghe host trong config, mặc định 127.0.0.1), `withAuth` (only nếu config yêu cầu API key).

Streaming: hỗ trợ SSE (`stream:true`) — trả OpenAI `chat.completion.chunk`, kết thúc `data: [DONE]`. Profile có `streamMode:"disabled"` gửi `stream:true` → `400 streaming_not_supported` (không bao giờ tự đổi sang one-shot). Lỗi giữa chừng ghi thành `data: {error...}` rồi `[DONE]`. Xem bảng default stream mode trong README.

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

`internal/queue` — một queue cho **mỗi model profile id** (tạo lazy qua `queueFor(id, profile)`), config concurrency/maxQueue/queueTimeout lấy từ provider, **execTimeout lấy từ profile** (`timeoutSec`):

- `gate`: buffered channel `maxQueue` — cấp "ticket" khi vào hàng đợi. Hết chỗ → `429 provider_busy`.
- `slots`: buffered channel `concurrency` — cấp quyền chạy CLI thật. Đợi slot tiêu tốn queue timeout (`queueTimeoutSec`).
- Timeout exec (`ExecTimeout`) áp quanh `Run`/`RunStream` — mặc định profile 300s, 0 = unlimited.
- `SubmitStream` dùng chung gate/slot với `Submit` (cùng hàng đợi cho cả stream và không stream).
- Visitor không có "wait queue": nếu hết slot + hết chỗ trong gate thì từ chối ngay thay vì kẹt.
- `rebuildQueues` chỉ xóa cache queue (lazy recreate ở request kế).

Xem test `internal/queue/queue_test.go` để hiểu hành vi cạnh biên (timeout, cancel, full, stream deltas).

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
- `Config{ Server{Host,Port}, AutoStartServer, RequireAPIKey, APIKey, Providers{...}, Models[...], RetentionDays, DebugLogging }`.
- `Models` = danh sách `ModelProfile{ID, Provider, DisplayName, StreamMode(native|disabled), TimeoutSec, Enabled}` — thứ tự ổn định (thứ tự khai báo).
- Migration: đọc raw JSON; `port` top-level (v1) được nhét về `server.port`; thiếu `models` → seed 4 default profile (claude/opencode native, codex/gemini disabled); `models` khai báo rỗng `{}` → giữ rỗng (không seed lại).
- Mỗi `ProviderConfig`: `Concurrency`, `MaxQueue`, `QueueTimeoutSec`, `ExecTimeoutSec` (mặc định Concurrency=1, MaxQueue=10, QueueTimeout=120, ExecTimeout=0).
- `LoadFile` fallback defaults khi thiếu/hỏng; `Save` ghi atomic (tmp+rename); `SetModel`/`DeleteModel` validate id bằng regex `^[A-Za-z0-9][A-Za-z0-9._:\-]{0,63}$`.

---

## Lược đồ data

Các kiểu chủ chốt:

```text
provider.Request   { Model string;  Messages []Message }
provider.Message   { Role Role(system|user|assistant);  Content string }
provider.Invocation{ Exec string; Args []string; Stdin string; Env []string;
                     StreamParse StreamParseLine }   // nil = không stream được
provider.StreamEvent{ Text string }
provider.StreamParseLine func(line string) (delta string, done bool, err error)
provider.Result    { Content string }
provider.Error     { Code string; Provider string; Message string; Status int; Details string }
config.ModelProfile{ ID, Provider, DisplayName, StreamMode, TimeoutSec, Enabled }
core.ModelInput    { ID, Provider, DisplayName, StreamMode, TimeoutSeconds, Enabled }  // từ UI
core.ModelView     { ID, Provider, ProviderName, DisplayName, StreamMode, TimeoutSeconds, Enabled, Status, StatusKind, Ready }
api.ModelCheck     { Exists, Enabled, Provider string }   // cho /v1 gate
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

- `GetSnapshot()` trả `Snapshot{ ServerRunning, Host, Port, URL, ConfigURL, Providers[], Models[], Activity[] }`; UI render mỗi khi nhận `state` event.
- `Snapshot.Providers[i]`: `Alias, Name, Enabled, Installed, Version, Executable, Auth, Status, StatusKind, Concurrency...`.
- `Snapshot.Models[i]`: `ID, Provider, ProviderName, DisplayName, StreamMode, TimeoutSeconds, Enabled, Status, StatusKind, Ready`.
- Các action binding trong `app.go`: `RunChat`, `StartServer/StopServer/RestartServer`, `SetPort`, `PortInUse`, `SaveModel`, `DeleteModel`, `TestModel`, `TestProvider`, `SetAPIKeyEnabled`, `Configure`, `SaveProvider`.
- Dashboard = server panel (status chip, port edit, URL copy, start/stop/restart) + models table (test/edit/duplicate/delete) + activity.
- Xem `docs/ui-design-guide.md` cho quy ước CSS/layout.

---

Tài liệu liên quan: [README](../README.md) · [cli-discovery.md](cli-discovery.md) · [ui-design-guide.md](ui-design-guide.md).
