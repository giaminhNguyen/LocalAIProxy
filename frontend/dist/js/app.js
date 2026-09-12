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
  toastTimer = setTimeout(() => { t.hidden = true; }, Math.min(1600 + msg.length * 15, 4000));
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
let editingId = null;
let deleteTarget = null;

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
  window.runtime.EventsOn("close-requested", () => {
    $("close-backdrop").hidden = false;
    const safe = $("btn-cancel-close");
    if (safe) safe.focus();
  });

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

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      $("close-backdrop").hidden = true;
      $("firstrun-backdrop").hidden = true;
      $("model-backdrop").hidden = true;
      $("delete-backdrop").hidden = true;
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
  if (name === "settings") renderSettingsForm();
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
    try { await window.go.main.App.StartServer(); } catch (e) { toast("Could not start: " + friendlyErr(e)); }
    $("btn-start").disabled = false;
  });
  $("btn-stop").addEventListener("click", async () => { await window.go.main.App.StopServer(); });
  $("btn-restart").addEventListener("click", async () => {
    $("btn-restart").disabled = true;
    try { await window.go.main.App.RestartServer(); } catch (e) { toast("Could not restart: " + friendlyErr(e)); }
    $("btn-restart").disabled = false;
  });
  $("btn-refresh").addEventListener("click", refresh);
  $("btn-refresh-providers").addEventListener("click", () => refresh());

  $("btn-apply-port").addEventListener("click", applyPort);
  $("in-server-port").addEventListener("keydown", (e) => { if (e.key === "Enter") applyPort(); });
  $("in-server-port").addEventListener("input", () => { $("port-warning").hidden = true; $("port-error").hidden = true; });

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

  /* model modal */
  $("btn-add-model").addEventListener("click", () => openModelModal(false));
  $("btn-model-cancel").addEventListener("click", () => { $("model-backdrop").hidden = true; });
  $("btn-model-save").addEventListener("click", saveModel);
  $("sw-model-enabled").addEventListener("click", () => toggleSwitch($("sw-model-enabled")));

  /* delete confirm */
  $("btn-delete-cancel").addEventListener("click", () => { $("delete-backdrop").hidden = true; deleteTarget = null; });
  $("btn-delete-confirm").addEventListener("click", confirmDelete);
}

function toggleSwitch(sw) {
  const on = sw.getAttribute("aria-checked") === "true";
  sw.setAttribute("aria-checked", on ? "false" : "true");
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

/* ---- server panel ------------------------------------------------------ */

async function applyPort() {
  $("port-error").hidden = true;
  $("port-warning").hidden = true;
  const port = parseInt($("in-server-port").value, 10);
  if (!port || port < 1 || port > 65535) {
    $("port-error").textContent = "Port must be 1–65535.";
    $("port-error").hidden = false;
    return;
  }
  if (port === snapshot.port) return;
  if (port !== snapshot.port && await window.go.main.App.PortInUse(port)) {
    $("port-warning").hidden = false;
  }
  try {
    await window.go.main.App.SetPort(port);
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    toast("Port set to " + port);
  } catch (e) {
    $("port-error").textContent = friendlyErr(e);
    $("port-error").hidden = false;
    snapshot = await window.go.main.App.GetSnapshot();
    render();
  }
}

/* ---- model modal ------------------------------------------------------- */

function providerOptions() {
  const ps = snapshot.providers || [];
  let opts = "";
  for (const p of ps) {
    const note = !p.installed ? " (not installed)" : (!p.enabled ? " (disabled)" : "");
    opts += '<option value="' + esc(p.alias) + '">' + esc(p.name) + note + "</option>\n";
  }
  return opts || '<option value="">— no providers —</option>';
}

function openModelModal(mode, model) {
  editingId = null;
  $("model-id-error").hidden = true;
  $("model-form-error").hidden = true;

  const isEdit = mode === "edit";
  $("model-modal-title").textContent = isEdit ? "Edit model" : "Add model";
  $("in-model-provider").innerHTML = providerOptions();

  if (isEdit && model) {
    editingId = model.id;
    $("in-model-id").value = model.id;
    $("in-model-id").disabled = true;
    $("in-model-name").value = model.displayName || "";
    $("in-model-provider").value = model.provider;
    $("in-model-stream").value = model.streamMode === "disabled" ? "disabled" : "native";
    $("in-model-timeout").value = model.timeoutSeconds || 300;
    $("sw-model-enabled").setAttribute("aria-checked", model.enabled ? "true" : "false");
  } else {
    $("in-model-id").value = mode === "duplicate" && model ? (model.id + "-copy").slice(0, 63) : "";
    $("in-model-id").disabled = false;
    $("in-model-name").value = model && model.displayName ? model.displayName : "";
    const first = (snapshot.providers || []).find((p) => p.enabled && p.installed);
    $("in-model-provider").value = (model && model.provider) || (first ? first.alias : ((snapshot.providers || [])[0] || {}).alias || "");
    $("in-model-stream").value = model && model.streamMode ? model.streamMode : "native";
    $("in-model-timeout").value = model && model.timeoutSeconds ? model.timeoutSeconds : 300;
    $("sw-model-enabled").setAttribute("aria-checked", "true");
  }

  $("model-backdrop").hidden = false;
  if (isEdit || mode === "duplicate") {
    const f = isEdit ? $("in-model-name") : $("in-model-id");
    f.focus();
    if (mode === "duplicate") f.select();
  } else {
    $("in-model-id").focus();
  }
}

function fieldError(el, msg) {
  el.textContent = msg;
  el.hidden = !msg;
  return !!msg;
}

async function saveModel() {
  $("model-id-error").hidden = true;
  $("model-form-error").hidden = true;

  const id = $("in-model-id").value.trim();
  const sId = editingId || id;
  const timeout = parseInt($("in-model-timeout").value, 10);
  let bad = false;

  if (!/^[A-Za-z0-9][A-Za-z0-9._:\-]{0,63}$/.test(sId)) {
    bad = fieldError($("model-id-error"), 'Model ID must start with a letter or digit and use only letters, digits, " . _ : -" (max 64).');
  }
  if (!timeout || timeout < 1 || timeout > 86400) {
    fieldError($("model-form-error"), "Timeout must be 1–86400 seconds.");
    bad = true;
  }
  if (bad) return;

  const input = {
    id: sId,
    provider: $("in-model-provider").value,
    displayName: $("in-model-name").value.trim(),
    streamMode: $("in-model-stream").value,
    timeoutSeconds: timeout,
    enabled: $("sw-model-enabled").getAttribute("aria-checked") === "true",
  };
  try {
    await window.go.main.App.SaveModel(input);
    $("model-backdrop").hidden = true;
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    toast(editingId ? "Model " + sId + " updated" : "Model " + sId + " added");
  } catch (e) {
    fieldError($("model-form-error"), friendlyErr(e));
  }
}

async function confirmDelete() {
  if (!deleteTarget) return;
  try {
    await window.go.main.App.DeleteModel(deleteTarget);
    $("delete-backdrop").hidden = true;
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    toast("Model " + deleteTarget + " deleted");
  } catch (e) {
    $("delete-backdrop").hidden = true;
    toast("Could not delete: " + friendlyErr(e));
  }
  deleteTarget = null;
}

/* ---- rendering --------------------------------------------------------- */

function render() {
  renderHeader();
  renderDashboard();
  if (!$("view-providers").hidden) renderProviders();
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
  $("api-url").title = snapshot.url;
  const hint = running && snapshot.requireApiKey;
  $("api-hint").hidden = !hint;
  $("header-port").textContent = snapshot.host + ":" + snapshot.port;
  document.title = "Local AI Proxy — " + (running ? "running" : "stopped");
}

function renderDashboard() {
  renderServer();
  renderModels();
  renderActivity();
}

function renderServer() {
  $("in-server-port").value = snapshot.port;
}

function renderModels() {
  const body = $("models-body");
  const models = snapshot.models || [];
  if (!models.length) {
    body.innerHTML = '<tr class="empty-row"><td colspan="6">No models yet. Models route server requests to a provider CLI — add the first one.</td></tr>';
    return;
  }
  body.innerHTML = "";
  for (const m of models) {
    body.appendChild(modelRow(m));
  }
}

function modelRow(m) {
  const tr = document.createElement("tr");
  const testing = testRunning.has(m.id);
  const pill = testing
    ? '<span class="pill running"><span class="dot"></span>Testing…</span>'
    : statusPill(m.status, m.statusKind);

  tr.appendChild(td(
    '<div class="model-name"><span class="mono strong">' + esc(m.id) + "</span>" +
    (m.displayName ? '<span class="model-disp">' + esc(m.displayName) + "</span>" : "") + "</div>"
  ));
  tr.appendChild(td(
    '<div class="model-backend"><span class="mono">' + esc(m.provider) + "</span>" +
    '<span class="model-disp">' + esc(m.providerName) + "</span></div>"
  ));
  tr.appendChild(td(streamChip(m)));
  tr.appendChild(td('<span class="mono">' + (m.timeoutSeconds ? m.timeoutSeconds + "s" : "none") + "</span>"));
  tr.appendChild(td(pill));
  tr.appendChild(td(
    '<div class="row-actions">' +
      '<button class="btn btn-ghost btn-sm model-action test" data-model="' + esc(m.id) + '" ' +
        (m.enabled && !testing && m.ready ? "" : 'disabled title="Not ready to test"') + ">" +
        (testing ? "Testing…" : "Test") + "</button>" +
      '<button class="btn btn-ghost btn-sm model-action edit" data-model="' + esc(m.id) + '" title="Edit">Edit</button>' +
      '<button class="btn btn-ghost btn-sm model-action dup" data-model="' + esc(m.id) + '" title="Duplicate">Duplicate</button>' +
      '<button class="btn btn-danger btn-sm model-action del" data-model="' + esc(m.id) + '" title="Delete">Delete</button>' +
    "</div>"
  ));
  return tr;
}

function streamChip(m) {
  const native = m.streamMode === "native";
  return '<span class="stream-chip ' + (native ? "native" : "disabled") + '">' +
    (native ? "Native" : "Disabled") + "</span>";
}

function renderActivity() {
  const body = $("activity-body");
  const items = snapshot.activity || [];
  if (!items.length) {
    body.innerHTML = '<tr class="empty-row"><td colspan="5">No requests yet. Send one, or press Test on a model.</td></tr>';
    return;
  }
  body.innerHTML = "";
  for (const a of items.slice(0, 20)) {
    const tr = document.createElement("tr");
    tr.appendChild(td('<span class="mono">' + esc(a.time) + "</span>"));
    tr.appendChild(td(esc(a.provider)));
    tr.appendChild(td(a.model ? esc(a.model) : "—"));
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
  const open = {};
  for (const item of list.querySelectorAll(".provider-item.open")) open[item.id] = true;

  const ps = snapshot.providers || [];
  if (!ps.length) {
    list.innerHTML =
      '<div class="panel"><div class="panel-body"><span class="hint">No supported CLI detected. Install one, then press Scan again.</span></div></div>';
    return;
  }

  list.innerHTML = "";
  for (const p of ps) {
    const item = providerItem(p);
    if (open["prov-" + p.alias]) {
      item.classList.add("open");
      const det = item.querySelector(".pdetail");
      if (det) det.hidden = false;
    }
    list.appendChild(item);
  }
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
      '<button class="btn btn-sm test-btn" data-alias="' + esc(p.alias) + '"' +
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
    def("State", p.status);

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
  wrap.appendChild(enabledRow);

  wireAfterRender(wrap, p);
  return wrap;
}

function wireAfterRender(wrap, p) {
  const testBtn = wrap.querySelector(".test-btn");
  if (testBtn) testBtn.addEventListener("click", () => runTest(p.alias));
  const copyBtn = wrap.querySelector(".copy-alias");
  if (copyBtn) copyBtn.addEventListener("click", () => copyText(p.alias, "Copied alias: " + p.alias));
  const enableBtn = wrap.querySelector("[data-enable]");
  if (enableBtn) enableBtn.addEventListener("click", () => toggleProvider(p.alias));
}

async function toggleProvider(alias) {
  const p = snapshot.providers.find((x) => x.alias === alias);
  if (!p) return;
  try {
    await window.go.main.App.SaveProvider(alias, { enabled: !p.enabled });
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    renderProviders();
    toast(p.enabled ? alias + " disabled" : alias + " enabled");
  } catch (e) {
    toast("Could not update provider: " + friendlyErr(e));
  }
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

/* event delegation for dynamic content */
document.addEventListener("click", (e) => {
  const testBtn = e.target.closest(".test-btn");
  if (testBtn) { runTest(testBtn.dataset.alias); return; }

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

  const action = e.target.closest(".model-action");
  if (action) {
    const id = action.dataset.model;
    if (action.classList.contains("test")) testModel(id);
    else if (action.classList.contains("edit")) {
      const m = (snapshot.models || []).find((x) => x.id === id);
      if (m) openModelModal("edit", m);
    } else if (action.classList.contains("dup")) {
      const m = (snapshot.models || []).find((x) => x.id === id);
      if (m) openModelModal("duplicate", m);
    } else if (action.classList.contains("del")) {
      const m = (snapshot.models || []).find((x) => x.id === id);
      deleteTarget = id;
      $("delete-model-name").textContent = m ? "Model " + m.id + (m.displayName ? " (" + m.displayName + ")" : "") + " will be removed. Requests using it will fail until you add it again." : "";
      $("delete-backdrop").hidden = false;
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

async function testModel(id) {
  if (testRunning.has(id)) return;
  testRunning.add(id);
  render();
  try {
    const res = await window.go.main.App.TestModel(id);
    snapshot = await window.go.main.App.GetSnapshot();
    toast(res && res.passed
      ? "Model " + id + " OK — " + res.latencyMs + "ms"
      : "Model " + id + " failed: " + friendlyErr(res && res.message ? res.message : "request failed"));
  } catch (e) {
    toast("Test failed: " + friendlyErr(e));
  } finally {
    testRunning.delete(id);
    render();
  }
}

/* ---- settings table ---------------------------------------------------- */

function wireSettings() {
  for (const id of ["sw-autostart", "sw-auth", "sw-logs", "sw-debug"]) {
    $(id).addEventListener("click", () => {
      toggleSwitch($(id));
      toggleLinked(id);
    });
  }
  for (const id of ["sw-claude", "sw-codex", "sw-gemini", "sw-opencode"]) {
    $(id).addEventListener("click", () => toggleSwitch($(id)));
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
    snapshotRefresh();
    $("settings-status").textContent = "Saved.";
    autoClearStatus();
  } catch (e) {
    $("settings-status").textContent = "Error: " + friendlyErr(e);
  }
}

async function snapshotRefresh() {
  snapshot = await window.go.main.App.GetSnapshot();
  render();
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