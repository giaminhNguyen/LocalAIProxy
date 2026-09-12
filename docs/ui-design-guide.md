# UI design guide

Design intent: a **compact desktop developer utility** — dense rows, hairline dividers, one calm accent, status never conveyed by color alone.

## Rules
1. **Density first.** Every control is 32px or smaller; panels use 4px grid spacing inherited from `--space`. No cards-in-cards; fields sit in flat rows with a floating label.
2. **Hairlines, not shadows.** Structure comes from `1px rgba(255,255,255,.07)` borders. Shadows reserved for modals.
3. **Status = icon + text.** A status is a dot + label (`Ready`, `Disabled`, `Auth required`, `Not installed`, `Auth unknown`). The dot tint is a support signal, never the only one. Keep red/green/amber meanings conventional.
4. **One accent.** `--accent` (calm cyan) is used for the "running" state and focus rings only; primary actions are neutral white-on-dark.
5. **Monospace for values.** URLs, aliases (`claude`, `codex`…), ports, latencies render in `--font-mono`.
6. **Keyboard-first.** Tabs are switchable with Ctrl+Tab; every focusable element has a visible `:focus-visible` ring; Esc closes dialogs.
7. **Reduced motion respected.** `prefers-reduced-motion: reduce` disables all transitions.

## Layout
- 46px header: brand mark + name left; server status chip (running/stopped) right, with port.
- 38px tab bar: Dashboard / Providers / Settings (Ctrl+Tab to switch).
- Content column (`max-width: 880px`): panels stack with 14px gutters.
- Dashboard: API URL panel (+ copy), providers table (status · name · alias chip · Test), recent activity table.
- Providers: one flat `provider-item` per provider; chevron expands details (paths, version, auth, advanced numbers, Test history, Enable/Disable).
- Settings: server group (autostart, port, API-key auth, key with copy/generate, logging, debug), per-provider advanced grid (concurrency / max queued / queue timeout / exec timeout), danger zone (restore defaults, re-show welcome).

## Copy
- Headlines: title case, no exclamation.
- Buttons: imperative single verbs (`Test`, `Refresh`, `Save`).
- Empty activity: "No requests yet. Send one, or press Test on a provider."
- Privacy-first phrasing for the API-key and welcome copy: everything stays on-loopback, no telemetry, no accounts created.

## Implementation notes
- All UI state comes from one `GetSnapshot()` call plus `state` events; `app.js` re-renders idempotently.
- Clipboard via `window.runtime.ClipboardSetText` (Wails v2).
- No framework, no build step: the three files in `frontend/dist` are embedded via `go:embed`.