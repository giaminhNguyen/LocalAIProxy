# CLI discovery notes

Verified live on this machine (Windows 11) — invocation flags below come from each CLI's own `--help`, not from memory.

## Providers

### claude — `C:\Users\ming\.local\bin\claude.exe` (v2.1.269)
- **Login detection**: `claude auth status` prints JSON; `loggedIn == true` means authenticated (`auth_method` e.g. `claude.ai`).
- **Invoke**: `claude -p <prompt> --output-format json --permission-mode plan --permission-prompts none`
- **Parse**: JSON, field `result` (final message text). Pure stdout, no ANSI noise.
- Notes: `--verbose` adds stderr trace lines; keep disabled. Model(s): `opus`/`sonnet`/`haiku` per configured account; determined at runtime by Claude — Local AI Proxy does not choose.

### codex — `C:\Users\ming\AppData\Local\Programs\OpenAI\Codex\bin\codex.exe` (0.153.4)
- **Login detection**: `codex login status` — exit 0 and/or "Logged in using …" text.
- **Invoke** (ephemeral, sandboxed, read-only):
  ```
  codex exec --json --ephemeral --sandbox read-only --skip-git-repo-check --output-last-message <tmpfile> -     (prompt on stdin)
  ```
- **Parse**: read `<tmpfile>` (last message). Fallback: scan stdout JSONL events for the final `message` payload.
- Notes: `--ephemeral` leaves no history; `--sandbox read-only` keeps it non-mutating (denied write actions are refused, never sent to the model). Older codex builds had `--ask-for-approval never`; 0.154+ removed it — read-only auto-denies prompts so no flag is needed. Prompt is streamed on stdin so it works for arbitrary lengths.

### gemini — not installed on this machine (planned binary name from the `gemini` CLI, e.g. `gemini-cli`)
- **Login detection**: `gemini auth status` (exit code / status text). Unverified locally — treated as **Auth unknown** when inconclusive.
- **Invoke**: `gemini -p <prompt>` (print-only mode).
- **Parse**: stdout text; strip ANSI control sequences.
- Real-provider test for gemini is **skipped** until the CLI is installed here.

### opencode — `C:\Users\ming\.opencode\bin\opencode.exe` (1.18.30)
- **Login detection**: `opencode auth list` — a line matching `0 credentials` means auth required; otherwise any credential listed means authenticated.
- **Invoke**: `opencode run --format default <prompt>` with env `NO_COLOR=1`.
- **Parse**: stdout text; strip ANSI control sequences.
- Notes: currently `auth list` reports `0 credentials` on this machine → UI shows **Auth required**.

## Cross-cutting

- Every provider invocation runs as its own subprocess **in the working directory of the Local AI Proxy app**, with `NO_COLOR=1`, `CLICOLOR=0` set, and stderr (sanitized, truncated) available to the GUI only — raw stderr is never forwarded to API clients.
- Executables are resolved via `exec.LookPath`; when the same name exists both in PATH and in the CLI's canonical install dir, the canonical install dir wins (matches the manuall installs above).
- MITM/CA: `claude` installs its own local cert store; do **not** route it through an HTTP proxy.
- All outputs are capped at 8 MiB per request; overflow truncates and marks the result.