/* Local AI Proxy — UI logic. Vanilla JS, no framework. */

"use strict";

const $ = (id) => document.getElementById(id);

/* ---- clipboard --------------------------------------------------------- */

async function copyText(text, okMessage) {
  try {
    if (window.runtime && window.runtime.ClipboardSetText) {
      await window.runtime.ClipboardSetText(text);
    } else {
      await navigator.clipboard.writeText(text);
    }
    toast(okMessage || "Copied");
  } catch (e) {
    toast("Could not copy");
  }
}

let toastTimer = null;
function toast(msg) {
  const t = $("toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 1600);
}

/* ---- icons ------------------------------------------------------------- */

const ICONS = {
  copy: '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="11" height="11" rx="2"></rect><path d="M5 15V5a2 2 0 0 1 2-2h10"></path></svg>',
  chevron: '<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"></path></svg>',
};

/* ---- app state --------------------------------------------------------- */

let snapshot = null;
let settings = null;
let apiKey = "";
let testRunning = new Set();

/* ---- main -------------------------------------------------------------- */

async function init() {
  if (!window.go || !window.go.main || !window.go.main.App) {
    document.body.innerHTML =
      '<div style="padding:40px;color:#a2a9b4;font-family:sans-serif">Local AI Proxy must run inside its desktop shell. Open LocalAIProxy.exe instead of this file.</div>';
    return;
  }

  wireTabs();
  wireButtons();
  window.runtime.EventsOn("state", (snap) => { snapshot = snap; render(); });
  window.runtime.EventsOn("close-requested", () => { $("close-backdrop").hidden = false; });

  snapshot = await window.go.main.App.GetSnapshot();
  settings = await window.go.main.App.GetConfig();
  try { apiKey = await window.go.main.App.GetAPIKey(); } catch (e) { apiKey = ""; }

  render();
  wireSettings();
  renderSettingsForm();

  if (await window.go.main.App.IsFirstRun()) {
    $("firstrun-backdrop").hidden = false;
    $("btn-gotit").focus();
  }

  // keyboard: Esc closes dialogs
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      $("close-backdrop").hidden = true;
      $("firstrun-backdrop").hidden = true;
    }
  });
}

/* ---- tabs -------------------------------------------------------------- */

function wireTabs() {
  const tabs = ["dashboard", "providers", "settings"];
  for (const name of tabs) {
    $("tab-" + name).addEventListener("click", () => switchTab(name));
  }
  document.addEventListener("keydown", (e) => {
    const idx = ["dashboard", "providers", "settings"].indexOf(document.querySelector(".tab.is-active").id.replace("tab-", ""));
    const dir = e.ctrlKey ? (e.key === "Tab" ? (e.shiftKey ? -1 : 1) : 0) : 0;
    if (dir && idx >= 0) {
      e.preventDefault();
      const next = (idx + dir + 3) % 3;
      switchTab(["dashboard", "providers", "settings"][next]);
    }
  });
}

function switchTab(name) {
  if (name === "providers") renderProviders();
  if (name === "settings") { renderSettingsForm(); }
  for (const t of ["dashboard", "providers", "settings"]) {
    const tab = $("tab-" + t);
    tab.classList.toggle("is-active", t === name);
    tab.setAttribute("aria-selected", t === name ? "true" : "false");
    $("view-" + t).hidden = t !== name;
  }
}

/* ---- wiring ------------------------------------------------------------ */

function wireButtons() {
  $("btn-copy-url").addEventListener("click", () => copyText(($("api-url").textContent || "").trim(), "Copied API URL"));
  $("btn-start").addEventListener("click", async () => {
    $("btn-start").disabled = true;
    try { await window.go.main.App.StartServer(); } catch (e) { toast(String(e)); }
    $("btn-start").disabled = false;
  });
  $("btn-stop").addEventListener("click", async () => { await window.go.main.App.StopServer(); });
  $("btn-restart").addEventListener("click", async () => {
    $("btn-restart").disabled = true;
    try { await window.go.main.App.RestartServer(); } catch (e) { toast("Could not restart: " + friendlyErr(e)); }
    $("btn-restart").disabled = false;
  });
  $("btn-refresh").addEventListener("click", async () => { await refresh(); });
  $("btn-refresh-providers").addEventListener("click", async () => { await refresh(); });

  $("btn-gotit").addEventListener("click", async () => {
    $("firstrun-backdrop").hidden = true;
    try { await window.go.main.App.DismissFirstRun(); } catch (e) {}
  });
  $("btn-cancel-close").addEventListener("click", () => { $("close-backdrop").hidden = true; });
  $("btn-confirm-close").addEventListener("click", async () => { await window.go.main.App.ConfirmClose(); });

  $("btn-copy-key").addEventListener("click", () => copyText(apiKey || "", "Copied API key"));
  $("btn-gen-key").addEventListener("click", async () => {
    try { apiKey = await window.go.main.App.GenerateAPIKey(); renderSettingsForm(); toast("New key generated"); }
    catch (e) { toast("Could not generate key"); }
  });

  $("btn-restore").addEventListener("click", async () => {
    await window.go.main.App.RestoreDefaults();
    settings = await window.go.main.App.GetConfig();
    apiKey = await window.go.main.App.GetAPIKey();
    renderSettingsForm();
    toast("Restored defaults");
  });
  $("btn-reset-firstrun").addEventListener("click", async () => {
    await window.go.main.App.ResetFirstRun();
    toast("Welcome guide will show again on next launch");
  });
  $("btn-save-settings").addEventListener("click", saveSettings);
}

async function refresh() {
  await window.go.main.App.Refresh();
  snapshot = await window.go.main.App.GetSnapshot();
  render();
  toast("Scanned providers");
}

function friendlyErr(e) {
  const s = String(e && e.message ? e.message : e);
  return s.replace(/^Error:\s*/, "");
}

/* ---- rendering --------------------------------------------------------- */

function render() {
  renderHeader();
  renderDashboard();
}

function renderHeader() {
  const chip = $("server-chip");
  const text = $("server-chip-text");
  const running = snapshot.serverRunning;
  chip.className = "chip " + (running ? "chip-running" : "chip-stopped");
  text.textContent = running ? "Server running · port " + snapshot.port : "Server stopped";
  $("btn-start").hidden = running;
  $("btn-stop").hidden = !running;
  $("btn-restart").hidden = !running;
  $("api-url").textContent = snapshot.url;
  const hint = running && snapshot.requireApiKey;
  $("api-hint").hidden = !hint;
  document.title = "Local AI Proxy — " + (running ? "running" : "stopped");
}

function renderDashboard() {
  const body = $("providers-dash-body");
  body.innerHTML = "";
  for (const p of snapshot.providers) {
    const tr = document.createElement("tr");
    const pill = statusPill(p.status, p.statusKind, testRunning.has(p.alias));
    tr.appendChild(td(pill));
    tr.appendChild(td('<span class="pname">' + esc(p.name) + "</span>"));
    tr.appendChild(td(aliasCell(p.alias)));
    tr.appendChild(td(testCell(p, "dash")));
    body.appendChild(tr);
  }
  renderActivity();
}

function renderActivity() {
  const body = $("activity-body");
  const items = snapshot.activity || [];
  if (!items.length) {
    body.innerHTML = '<tr class="empty-row"><td colspan="4">No requests yet. Send one, or press Test on a provider.</td></tr>';
    return;
  }
  body.innerHTML = "";
  for (const a of items.slice(0, 20)) {
    const tr = document.createElement("tr");
    tr.appendChild(td('<span class="mono">' + esc(a.time) + "</span>"));
    tr.appendChild(td(esc(a.provider)));
    tr.appendChild(td(activityStatus(a)));
    tr.appendChild(td(a.durationMs >= 0 ? a.durationMs + "ms" : "—"));
    body.appendChild(tr);
  }
}

function activityStatus(a) {
  if (a.ok) return '<span class="pill ok"><span class="dot"></span>' + esc(a.status) + "</span>";
  return '<span class="pill error"><span class="dot"></span>' + esc(shortStatus(a.status)) + "</span>";
}

function shortStatus(s) {
  return s.replace(/_/g, " ");
}

function renderProviders() {
  const list = $("provider-list");
  list.innerHTML = "";
  for (const p of snapshot.providers) {
    list.appendChild(providerItem(p));
  }
  // Expand the one whose test just ran / was last clicked
}

function providerItem(p) {
  const div = document.createElement("div");
  div.className = "provider-item";
  div.id = "prov-" + p.alias;

  const testState = testRunning.has(p.alias);
  const pill = statusPill(p.status, p.statusKind, testState);
  const canTest = p.enabled && p.installed && !testState;

  const row = document.createElement("div");
  row.className = "prow";
  row.innerHTML =
    '<div class="prow-main">' +
      '<div class="pname">' + pill + " " + esc(p.name) +
        '<span class="palias" title="model alias">' + esc(p.alias) + "</span></div>" +
      '<div class="pdesc">' + providerDesc(p) + "</div>" +
    "</div>" +
    '<div class="contact-line">' +
      '<button class="btn btn-sm test-btn ' + (p.installed ? "" : "") + '" data-alias="' + esc(p.alias) + '"' +
        (canTest ? "" : " disabled") + ">" + (testState ? "Testing…" : "Test") + "</button>" +
      '<button class="btn btn-ghost btn-sm copy-alias" data-alias="' + esc(p.alias) + '">' + ICONS.copy + " Copy alias</button>" +
      '<button class="chevron" aria-label="Details for ' + esc(p.name) + '" data-toggle="' + esc(p.alias) + '">' + ICONS.chevron + "</button>" +
    "</div>";
  div.appendChild(row);

  const detail = document.createElement("div");
  detail.className = "pdetail";
  detail.id = "pdetail-" + p.alias;
  detail.hidden = true;
  detail.appendChild(detailBody(p));
  div.appendChild(detail);

  return div;
}

function providerDesc(p) {
  if (!p.installed) return "Not detected on this machine.";
  return (p.version || "version unknown") + " · " + authLabel(p.auth);
}

function authLabel(a) {
  if (a === "detected") return "login detected";
  if (a === "required") return "needs login";
  return "login unknown";
}

function detailBody(p) {
  const wrap = document.createElement("div");

  const defs = document.createElement("dl");
  defs.className = "defs";
  defs.innerHTML =
    def("Executable", '<span class="code" title="' + esc(p.executable || "") + '">' + esc(truncMid(p.executable || "—", 60)) + "</span>") +
    def("Version", esc(p.version || "—")) +
    def("Authentication", authLabel(p.auth)) +
    def("State", statusWords(p.status));

  const adv = document.createElement("div");
  adv.className = "pdetails-adv";
  adv.innerHTML =
    advNum("Concurrency", p.concurrency) +
    advNum("Max queued", p.maxQueue) +
    advNum("Queue timeout (s)", p.queueTimeoutSec) +
    advNum("Execution timeout (s)", p.execTimeoutSec === 0 ? "Unlimited" : p.execTimeoutSec);

  wrap.appendChild(defs);

  if (!p.installed) {
    const mb = document.createElement("div");
    mb.className = "missing-box";
    mb.innerHTML =
      "<span><strong>" + esc(p.name) + " is not installed.</strong><br>Install it, then press Scan again in the Providers tab.</span>" +
      '<span class="mono hint small">' + esc(p.installedBy || "") + "</span>";
    wrap.appendChild(mb);
  }

  if (p.lastTest) {
    wrap.appendChild(lastTestBlock(p.lastTest));
  }

  const enabledRow = document.createElement("div");
  enabledRow.className = "contact-line";
  enabledRow.innerHTML =
    '<button class="btn btn-sm ' + (p.enabled ? "btn-primary" : "btn-ghost") + '" data-enable="' + esc(p.alias) + '">' +
    (p.enabled ? "Disable" : "Enable") + "</button>";
  enableRowAction(enabledRow.querySelector("[data-enable]"), p.alias);
  wrap.appendChild(enabledRow);

  wireAfterRender(wrap, p);
  return wrap;
}

function wireAfterRender(wrap, p) {
  const testBtn = wrap.querySelector(".test-btn");
  if (testBtn) testBtn.addEventListener("click", () => runTest(p.alias));
  const copyBtn = wrap.querySelector(".copy-alias");
  if (copyBtn) copyBtn.addEventListener("click", () => copyText(p.alias, "Copied alias: " + p.alias));
}

function enableRowAction(btn, alias) {
  btn.addEventListener("click", async () => {
    const p = snapshot.providers.find((x) => x.alias === alias);
    try {
      await window.go.main.App.SaveProvider(alias, { enabled: !p.enabled });
      snapshot = await window.go.main.App.GetSnapshot();
      render();
      renderProviders();
      toast(p.enabled ? alias + " disabled" : alias + " enabled");
    } catch (e) {
      toast("Could not update provider: " + friendlyErr(e));
    }
  });
}

function lastTestBlock(t) {
  const cls = t.passed ? "ok" : "fail";
  const headline = t.passed
    ? 'Test passed in ' + t.latencyMs + "ms"
    : 'Test failed (' + shortStatus(t.message || "error") + ")";
  return '<div class="test-result ' + cls + '">' +
    "<strong>" + esc(headline) + "</strong>" +
    (t.passed ? '<div class="detail">Response: "' + esc(truncMid(t.response || "", 120)) + '"</div>' : "") +
    (!t.passed ? '<div class="detail">' + esc(t.detail || t.message || "") + "</div>" : "") +
    "</div>";
}

function statusWords(status) {
  return status; // friendly label already
}

function def(k, v) {
  return '<div class="def"><dt>' + k + "</dt><dd>" + v + "</dd></div>";
}
function advNum(k, v) {
  return '<div class="adv-field"><label>' + k + '</label><code class="host-val">' + esc(String(v)) + "</code></div>";
}

function statusPill(status, kind, testing) {
  if (testing) return '<span class="pill running"><span class="dot"></span>Testing…</span>';
  const k = kind || "idle";
  return '<span class="pill ' + esc(k) + '"><span class="dot"></span>' + esc(status) + "</span>";
}

function aliasCell(alias) {
  return '<button class="copy-alias-chip" data-alias="' + esc(alias) + '" title="Copy alias">' +
    '<span class="alias-chip">' + esc(alias) + "</span></button>";
}

function testCell(p, where) {
  const idle = testRunning.has(p.alias);
  const canTest = p.enabled && p.installed && !idle;
  const lat = p.lastTest ? " · " + p.lastTest.latencyMs + "ms" : "";
  return '<div class="test-cell">' +
    '<span class="test-latency">' + (p.lastTest ? (p.lastTest.passed ? "Pass" : "Fail") + lat : "") + "</span>" +
    '<button class="btn btn-ghost btn-sm test-btn" data-alias="' + esc(p.alias) + '"' + (canTest ? "" : " disabled") + ">" +
    (idle ? "Testing…" : "Test") + "</button></div>";
}

/* event delegation for dynamic content */
document.addEventListener("click", (e) => {
  const testBtn = e.target.closest(".test-btn");
  if (testBtn) { runTest(testBtn.dataset.alias); return; }
  const copyChip = e.target.closest(".copy-alias-chip");
  if (copyChip) { copyText(copyChip.dataset.alias, "Copied alias: " + copyChip.dataset.alias); return; }
  const copyAlias = e.target.closest(".copy-alias");
  if (copyAlias) { copyText(copyAlias.dataset.alias, "Copied alias: " + copyAlias.dataset.alias); return; }
  const chev = e.target.closest(".chevron");
  if (chev) {
    const detail = $("pdetail-" + chev.dataset.toggle);
    const item = $("prov-" + chev.dataset.toggle);
    if (detail) {
      detail.hidden = !detail.hidden;
      item.classList.toggle("open", !detail.hidden);
    }
    return;
  }
});

async function runTest(alias) {
  if (testRunning.has(alias)) return;
  testRunning.add(alias);
  render();
  renderProviders();
  try {
    await window.go.main.App.TestProvider(alias);
    snapshot = await window.go.main.App.GetSnapshot();
  } catch (e) {
    toast("Test failed: " + friendlyErr(e));
  } finally {
    testRunning.delete(alias);
    render();
    renderProviders();
  }
}

/* ---- settings table ---------------------------------------------------- */

function wireSettings() {
  for (const id of ["sw-autostart", "sw-auth", "sw-logs", "sw-debug"]) {
    $(id).addEventListener("click", () => {
      const sw = $(id);
      sw.setAttribute("aria-checked", sw.getAttribute("aria-checked") === "true" ? "false" : "true");
      toggleLinked(id);
    });
  }
  for (const id of ["sw-claude", "sw-codex", "sw-gemini", "sw-opencode"]) {
    $(id).addEventListener("click", () => {
      const sw = $(id);
      sw.setAttribute("aria-checked", sw.getAttribute("aria-checked") === "true" ? "false" : "true");
    });
  }
}

function toggleLinked(id) {
  if (id === "sw-auth") $("apikey-row").hidden = $("sw-auth").getAttribute("aria-checked") !== "true";
  if (id === "sw-logs") $("retention-row").hidden = $("sw-logs").getAttribute("aria-checked") !== "true";
}

function renderSettingsForm() {
  if (!settings) return;
  $("sw-autostart").setAttribute("aria-checked", settings.autoStartServer ? "true" : "false");
  $("in-port").value = settings.port;
  $("sw-auth").setAttribute("aria-checked", settings.requireApiKey ? "true" : "false");
  toggleLinked("sw-auth");
  $("apikey-val").textContent = apiKey || "—";
  for (const alias of ["claude", "codex", "gemini", "opencode"]) {
    const p = (settings.providers || {})[alias];
    $("sw-" + alias).setAttribute("aria-checked", p && p.enabled ? "true" : "false");
  }
  $("sw-logs").setAttribute("aria-checked", settings.saveLogsToDisk ? "true" : "false");
  toggleLinked("sw-logs");
  $("in-retention").value = settings.retentionDays || 7;
  $("sw-debug").setAttribute("aria-checked", settings.debugLogging ? "true" : "false");

  // advanced per-provider grid
  const grid = $("adv-grid");
  grid.innerHTML = "";
  for (const alias of ["claude", "codex", "gemini", "opencode"]) {
    const p = (settings.providers || {})[alias] || {};
    grid.appendChild(advCard(alias, p));
  }
  $("settings-status").textContent = "";
}

function advCard(alias, p) {
  const card = document.createElement("div");
  card.className = "adv-card";
  card.innerHTML =
    '<div class="aliastitle"><span class="palias">' + esc(alias) + "</span><span>" + cap(alias) + "</span></div>" +
    numField("Concurrency", "concurrency-" + alias, p.concurrency || 1, 1, 64) +
    numField("Max queued", "maxQueue-" + alias, p.maxQueue ?? 10, 0, 10000) +
    numField("Queue timeout (s)", "queueTimeoutSec-" + alias, p.queueTimeoutSec ?? 120, 0, 3600) +
    numField("Execution timeout (s)", "execTimeoutSec-" + alias, p.execTimeoutSec ?? 0, 0, 86400);
  return card;
}

function numField(labelText, id, value, min, max) {
  return '<div class="adv-num"><label for="' + id + '">' + labelText + '</label>' +
    '<input type="number" class="input" id="' + id + '" min="' + min + '" max="' + max + '" value="' + value + '" aria-label="' + labelText + ' for ' + cap(id.split("-").slice(-1)[0]) + '"></div>';
}

function cap(s) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

async function saveSettings() {
  const port = parseInt($("in-port").value, 10);
  if (!port || port < 1 || port > 65535) {
    $("settings-status").textContent = "Port must be 1–65535.";
    return;
  }
  const providers = {};
  for (const alias of ["claude", "codex", "gemini", "opencode"]) {
    providers[alias] = {
      enabled: $("sw-" + alias).getAttribute("aria-checked") === "true",
      concurrency: clampInt($("concurrency-" + alias).value, 1, 64, 1),
      maxQueue: clampInt($("maxQueue-" + alias).value, 0, 10000, 10),
      queueTimeoutSec: clampInt($("queueTimeoutSec-" + alias).value, 0, 3600, 120),
      execTimeoutSec: clampInt($("execTimeoutSec-" + alias).value, 0, 86400, 0),
    };
  }
  try {
    await window.go.main.App.Configure({
      port,
      autoStartServer: $("sw-autostart").getAttribute("aria-checked") === "true",
      saveLogsToDisk: $("sw-logs").getAttribute("aria-checked") === "true",
      retentionDays: clampInt($("in-retention").value, 1, 365, 7),
      debugLogging: $("sw-debug").getAttribute("aria-checked") === "true",
      providers,
    });
    await window.go.main.App.SetAPIKeyEnabled($("sw-auth").getAttribute("aria-checked") === "true");
    settings = await window.go.main.App.GetConfig();
    apiKey = await window.go.main.App.GetAPIKey();
    $("apikey-val").textContent = apiKey || "—";
    renderSettingsForm();
    $("settings-status").textContent = "Saved.";
    autoClearStatus();
  } catch (e) {
    $("settings-status").textContent = "Error: " + friendlyErr(e);
  }
}

function autoClearStatus() {
  clearTimeout(autoClearStatus._t);
  autoClearStatus._t = setTimeout(() => { $("settings-status").textContent = ""; }, 2500);
}

function clampInt(v, min, max, def) {
  const n = parseInt(v, 10);
  if (isNaN(n)) return def;
  return Math.max(min, Math.min(max, n));
}

/* ---- helpers ----------------------------------------------------------- */

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

function td(html) {
  const el = document.createElement("td");
  el.innerHTML = html;
  return el;
}

function truncMid(s, max) {
  if (s.length <= max) return s;
  const half = Math.floor((max - 1) / 2);
  return s.slice(0, half) + "…" + s.slice(-half);
}

init();