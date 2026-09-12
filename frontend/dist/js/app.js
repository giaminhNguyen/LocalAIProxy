/* Local AI Proxy — UI logic. Vanilla JS, no framework. */

"use strict";

const $ = (id) => document.getElementById(id);

/* ---- i18n -------------------------------------------------------------- */

const I18N = {
  en: {
    "lang.aria": "Language",
    "tab.dashboard": "Dashboard",
    "tab.providers": "Providers",
    "tab.settings": "Settings",

    "dash.title": "Dashboard",
    "dash.subtitle": "Model profiles route to your installed CLIs, exposed as an OpenAI-compatible API.",

    "server.title": "Local server",
    "server.chip.starting": "Starting…",
    "server.chip.running": "Server running · port {p}",
    "server.chip.stopped": "Server stopped",
    "server.running": "running",
    "server.stopped": "stopped",
    "server.url": "API URL",
    "server.port": "Port",
    "server.apply": "Apply",
    "server.portInUse": "That port is already in use on this machine.",
    "server.portRange": "Port must be 1–65535.",
    "server.copy": "Copy URL",
    "server.start": "Start server",
    "server.stop": "Stop server",
    "server.restart": "Restart",
    "server.refresh": "Refresh",
    "server.apiKeyHint": "An API key is required — copy it from Settings and use it as <code>Authorization: Bearer &lt;key&gt;</code>.",
    "server.copied": "Copied API URL",
    "server.portSet": "Port set to {p}",
    "server.startFail": "Could not start: {e}",
    "server.restartFail": "Could not restart: {e}",

    "models.title": "Models",
    "models.note": "Each row is a model your clients can request with <code>model</code>.",
    "models.add": "+ Add model",
    "models.col.model": "Model",
    "models.col.backend": "Backend",
    "models.col.stream": "Streaming",
    "models.col.timeout": "Timeout",
    "models.col.status": "Status",
    "models.col.actions": "Actions",
    "models.empty": "No models yet. Models route server requests to a provider CLI — add the first one.",
    "models.stream.native": "Native",
    "models.stream.disabled": "Disabled",
    "models.timeout.unlimited": "none",
    "models.test": "Test",
    "models.testing": "Testing…",
    "models.edit": "Edit",
    "models.duplicate": "Duplicate",
    "models.delete": "Delete",
    "models.addTitle": "Add model",
    "models.editTitle": "Edit model",
    "models.id": "Model ID",
    "models.idPlaceholder": "e.g. claude-sonnet",
    "models.idHint1": "What clients send as",
    "models.idHint2": ". Letters, digits,",
    "models.idHint3": ".",
    "models.idError": 'Model ID must start with a letter or digit and use only letters, digits, " . _ : -" (max 64).',
    "models.timeoutError": "Timeout must be 1–86400 seconds.",
    "models.name": "Display name",
    "models.opt": "optional",
    "models.namePlaceholder": "e.g. Claude Sonnet (fast)",
    "models.backend": "Backend",
    "models.timeout": "Timeout (seconds)",
    "models.stream": "Streaming",
    "models.stream.nativeLabel": "Native — tokens stream as SSE chunks",
    "models.stream.disabledLabel": "Disabled — one-shot reply, stream=true is rejected",
    "models.streamHint1": "\u201CDisabled\u201D models answer in one JSON response and refuse",
    "models.streamHint2": "with",
    "models.streamHint3": "rather than fake-token near-real-time output.",
    "models.enabled": "Enabled",
    "models.enabledHint": "Disabled models disappear from <code>/v1/models</code> and refuse requests.",
    "models.save": "Save model",
    "models.added": "Model {id} added",
    "models.updated": "Model {id} updated",
    "models.deleteTitle": "Delete model?",
    "models.deleteMsg": "Model {id}{disp} will be removed. Requests using it will fail until you add it again.",
    "models.deleteConfirm": "Delete",
    "models.deleted": "Model {id} deleted",

    "activity.title": "Recent activity",
    "activity.col.time": "Time",
    "activity.col.provider": "Provider",
    "activity.col.model": "Model",
    "activity.col.status": "Status",
    "activity.col.duration": "Duration",
    "activity.empty": "No requests yet. Send one, or press Test on a model.",

    "providers.title": "Providers",
    "providers.subtitle": "Each provider runs through its own CLI, using that CLI's existing login.",
    "providers.scan": "Scan again",
    "providers.empty": "No supported CLI detected. Install one, then press Scan again.",
    "providers.notInstalled": "Not detected on this machine.",
    "providers.auth.detected": "login detected",
    "providers.auth.required": "needs login",
    "providers.auth.unknown": "login unknown",
    "providers.enable": "Enable",
    "providers.disable": "Disable",
    "providers.copyAlias": "Copy alias",
    "providers.toggledOn": "{alias} enabled",
    "providers.toggledOff": "{alias} disabled",
    "providers.testPassed": "Test passed in {ms}ms",
    "providers.testFailed": "Test failed ({s})",
    "providers.response": "Response:",
    "providers.missingTitle": "{name} is not installed.",
    "providers.missingBody": "Install it, then press Scan again in the Providers tab.",
    "providers.state": "State",
    "providers.executable": "Executable",
    "providers.version": "Version",
    "providers.authentication": "Authentication",
    "providers.concurrency": "Concurrency",
    "providers.maxQueued": "Max queued",
    "providers.queueTimeout": "Queue timeout (s)",
    "providers.execTimeout": "Execution timeout (s)",
    "providers.unlimited": "Unlimited",

    "settings.title": "Settings",
    "settings.subtitle": "Changes apply to the local server and are saved automatically.",
    "settings.general": "General",
    "settings.lang": "Language",
    "settings.langHint": "Interface language.",
    "settings.autostart": "Start server automatically when app opens",
    "settings.autostartHint": "When off, the server stays stopped until you start it.",
    "settings.port": "Port",
    "settings.portHint": "Change it if the current port is in use.",
    "settings.host": "Host",
    "settings.hostHint": "Local only — never exposed to your network.",
    "settings.auth": "Authentication",
    "settings.requireKey": "Require API key",
    "settings.requireKeyHint": "Optional. Recommended when other tools connect to this machine.",
    "settings.apiKey": "Local API key",
    "settings.apiKeyHint": "Shown only here. Never logged.",
    "settings.copy": "Copy",
    "settings.generate": "Generate",
    "settings.providersTitle": "Providers",
    "settings.advanced": "Advanced",
    "settings.advancedHint": "Per-provider concurrency, queue and execution limits. One CLI process runs per request.",
    "settings.logging": "Logging",
    "settings.saveLogs": "Save logs to disk",
    "settings.saveLogsHint": "Sanitized — no prompts, keys or tokens. In-memory only when off.",
    "settings.retention": "Retention (days)",
    "settings.retentionHint": "Older log files are deleted automatically.",
    "settings.debug": "Debug logging",
    "settings.debugHint": "More detail, still sanitized.",
    "settings.danger": "Danger zone",
    "settings.restore": "Restore defaults",
    "settings.restoreHint": "Resets all settings back to factory values.",
    "settings.reset": "Reset",
    "settings.welcome": "Show welcome guide again",
    "settings.welcomeHint": "Reopens the first-run guide on next launch.",
    "settings.showAgain": "Show again",
    "settings.saveChanges": "Save changes",
    "settings.saved": "Saved.",
    "settings.error": "Error: {e}",

    "common.cancel": "Cancel",
    "common.gotIt": "Got it",
    "common.copied": "Copied",
    "common.copyFail": "Could not copy",

    "firstrun.title": "Welcome to Local AI Proxy",
    "firstrun.intro": "LocalAIProxy turns the AI CLIs installed on this machine into a local OpenAI-compatible API.",
    "firstrun.step1": "Start the local server",
    "firstrun.step2": "Copy the API URL",
    "firstrun.step3a": "Use a model from the Dashboard as",
    "firstrun.step3b": "— the defaults are",
    "firstrun.privacy": "Everything stays on this machine. LocalAIProxy never manages your provider logins.",

    "close.title": "Requests still running",
    "close.body": "One or more AI requests are still in progress. Closing now will cancel them.",
    "close.keepRunning": "Keep running",
    "close.cancelExit": "Cancel and exit",

    "toast.keyCopied": "Copied API key",
    "toast.keyGen": "New key generated",
    "toast.keyGenFail": "Could not generate key",
    "toast.restored": "Restored defaults",
    "toast.showWelcome": "Welcome guide will show again on next launch",
    "toast.scanned": "Scanned providers",
    "toast.copyAlias": "Copied alias: {alias}",
    "toast.testFailed": "Test failed: {e}",
    "toast.modelTestOk": "Model {id} OK — {ms}ms",
    "toast.modelTestFail": "Model {id} failed: {e}",
    "toast.deleteFail": "Could not delete: {e}",
    "toast.updateFail": "Could not update provider: {e}",

    "status.Ready": "Ready",
    "status.Disabled": "Disabled",
    "status.Not installed": "Not installed",
    "status.Backend not installed": "Backend not installed",
    "status.Auth required": "Auth required",
    "status.Auth unknown": "Auth unknown",

    "activity.ok": "success",
    "activity.error": "error",
    "activity.cancelled": "cancelled",
    "activity.timeout": "timeout",
    "activity.rate_limited": "rate limited",
    "activity.auth_error": "auth error",
    "activity.not_found": "not found",
  },

  vi: {
    "lang.aria": "Ngôn ngữ",
    "tab.dashboard": "Tổng quan",
    "tab.providers": "Nhà cung cấp",
    "tab.settings": "Cài đặt",

    "dash.title": "Tổng quan",
    "dash.subtitle": "Các model được định tuyến tới CLI đã cài trên máy, phơi ra dưới dạng API tương thích OpenAI.",

    "server.title": "Máy chủ cục bộ",
    "server.chip.starting": "Đang khởi động…",
    "server.chip.running": "Máy chủ đang chạy · cổng {p}",
    "server.chip.stopped": "Máy chủ đã dừng",
    "server.running": "đang chạy",
    "server.stopped": "đã dừng",
    "server.url": "API URL",
    "server.port": "Cổng",
    "server.apply": "Áp dụng",
    "server.portInUse": "Cổng này đang được chương trình khác sử dụng.",
    "server.portRange": "Cổng phải trong khoảng 1–65535.",
    "server.copy": "Sao chép URL",
    "server.start": "Khởi động",
    "server.stop": "Dừng máy chủ",
    "server.restart": "Khởi động lại",
    "server.refresh": "Làm mới",
    "server.apiKeyHint": "Bắt buộc có API key — sao chép từ Cài đặt và dùng dạng <code>Authorization: Bearer &lt;key&gt;</code>.",
    "server.copied": "Đã sao chép API URL",
    "server.portSet": "Đã đặt cổng {p}",
    "server.startFail": "Không khởi động được: {e}",
    "server.restartFail": "Không khởi động lại được: {e}",

    "models.title": "Mô hình",
    "models.note": "Mỗi hàng là một model mà client có thể gọi qua trường <code>model</code>.",
    "models.add": "+ Thêm model",
    "models.col.model": "Model",
    "models.col.backend": "Nền tảng",
    "models.col.stream": "Streaming",
    "models.col.timeout": "Thời gian chờ",
    "models.col.status": "Trạng thái",
    "models.col.actions": "Thao tác",
    "models.empty": "Chưa có model nào. Model định tuyến request tới CLI của nhà cung cấp — hãy thêm model đầu tiên.",
    "models.stream.native": "Native",
    "models.stream.disabled": "Tắt",
    "models.timeout.unlimited": "không giới hạn",
    "models.test": "Kiểm tra",
    "models.testing": "Đang kiểm tra…",
    "models.edit": "Sửa",
    "models.duplicate": "Nhân bản",
    "models.delete": "Xóa",
    "models.addTitle": "Thêm model",
    "models.editTitle": "Sửa model",
    "models.id": "Model ID",
    "models.idPlaceholder": "vd: claude-sonnet",
    "models.idHint1": "Chuỗi client gửi ở trường",
    "models.idHint2": ". Gồm chữ, số,",
    "models.idHint3": ".",
    "models.idError": 'Model ID phải bắt đầu bằng chữ hoặc số và chỉ dùng chữ, số, " . _ : -" (tối đa 64 ký tự).',
    "models.timeoutError": "Thời gian chờ phải trong khoảng 1–86400 giây.",
    "models.name": "Tên hiển thị",
    "models.opt": "tùy chọn",
    "models.namePlaceholder": "vd: Claude Sonnet (nhanh)",
    "models.backend": "Nền tảng",
    "models.timeout": "Thời gian chờ (giây)",
    "models.stream": "Streaming",
    "models.stream.nativeLabel": "Native — token phát trực tiếp dạng SSE",
    "models.stream.disabledLabel": "Tắt — trả lời trọn vẹn một lần, stream=true bị từ chối",
    "models.streamHint1": "Model \u201CTắt\u201D trả lời trong một JSON, từ chối",
    "models.streamHint2": "kèm",
    "models.streamHint3": "thay vì giả lập token theo thời gian thực.",
    "models.enabled": "Bật",
    "models.enabledHint": "Model bị tắt sẽ biến mất khỏi <code>/v1/models</code> và từ chối request.",
    "models.save": "Lưu model",
    "models.added": "Đã thêm model {id}",
    "models.updated": "Đã cập nhật model {id}",
    "models.deleteTitle": "Xóa model?",
    "models.deleteMsg": "Model {id}{disp} sẽ bị xóa. Request dùng model này sẽ lỗi cho tới khi bạn thêm lại.",
    "models.deleteConfirm": "Xóa",
    "models.deleted": "Đã xóa model {id}",

    "activity.title": "Hoạt động gần đây",
    "activity.col.time": "Thời gian",
    "activity.col.provider": "Nhà cung cấp",
    "activity.col.model": "Model",
    "activity.col.status": "Trạng thái",
    "activity.col.duration": "Thời lượng",
    "activity.empty": "Chưa có request nào. Gửi một request hoặc bấm Kiểm tra trên một model.",

    "providers.title": "Nhà cung cấp",
    "providers.subtitle": "Mỗi nhà cung cấp chạy qua CLI riêng, dùng đúng tài khoản đã đăng nhập của CLI đó.",
    "providers.scan": "Quét lại",
    "providers.empty": "Không phát hiện CLI nào được hỗ trợ. Hãy cài một CLI rồi bấm Quét lại.",
    "providers.notInstalled": "Không phát hiện trên máy này.",
    "providers.auth.detected": "đã đăng nhập",
    "providers.auth.required": "cần đăng nhập",
    "providers.auth.unknown": "chưa rõ trạng thái",
    "providers.enable": "Bật",
    "providers.disable": "Tắt",
    "providers.copyAlias": "Sao chép alias",
    "providers.toggledOn": "Đã bật {alias}",
    "providers.toggledOff": "Đã tắt {alias}",
    "providers.testPassed": "Kiểm tra thành công trong {ms}ms",
    "providers.testFailed": "Kiểm tra thất bại ({s})",
    "providers.response": "Trả lời:",
    "providers.missingTitle": "Chưa cài {name}.",
    "providers.missingBody": "Cài nó rồi bấm Quét lại trong tab Nhà cung cấp.",
    "providers.state": "Trạng thái",
    "providers.executable": "Tệp thực thi",
    "providers.version": "Phiên bản",
    "providers.authentication": "Xác thực",
    "providers.concurrency": "Xử lý song song",
    "providers.maxQueued": "Hàng đợi tối đa",
    "providers.queueTimeout": "Chờ trong hàng đợi (s)",
    "providers.execTimeout": "Thời gian chạy (s)",
    "providers.unlimited": "Không giới hạn",

    "settings.title": "Cài đặt",
    "settings.subtitle": "Thay đổi áp dụng cho máy chủ cục bộ và được lưu tự động.",
    "settings.general": "Chung",
    "settings.lang": "Ngôn ngữ",
    "settings.langHint": "Ngôn ngữ giao diện.",
    "settings.autostart": "Tự khởi động máy chủ khi mở ứng dụng",
    "settings.autostartHint": "Khi tắt, máy chủ đứng yên cho tới khi bạn khởi động thủ công.",
    "settings.port": "Cổng",
    "settings.portHint": "Đổi nếu cổng hiện tại đang bị chiếm dụng.",
    "settings.host": "Địa chỉ host",
    "settings.hostHint": "Chỉ chạy cục bộ — không bao giờ lộ ra mạng.",
    "settings.auth": "Xác thực",
    "settings.requireKey": "Yêu cầu API key",
    "settings.requireKeyHint": "Tùy chọn. Nên bật khi có công cụ khác kết nối tới máy này.",
    "settings.apiKey": "API key cục bộ",
    "settings.apiKeyHint": "Chỉ hiển thị ở đây. Không bao giờ ghi log.",
    "settings.copy": "Sao chép",
    "settings.generate": "Tạo mới",
    "settings.providersTitle": "Nhà cung cấp",
    "settings.advanced": "Nâng cao",
    "settings.advancedHint": "Giới hạn xử lý song song, hàng đợi và thời gian chạy cho từng nhà cung cấp. Mỗi request một tiến trình CLI.",
    "settings.logging": "Ghi log",
    "settings.saveLogs": "Lưu log ra đĩa",
    "settings.saveLogsHint": "Đã làm sạch — không có prompt, key hay token. Khi tắt chỉ giữ trong bộ nhớ.",
    "settings.retention": "Lưu giữ (ngày)",
    "settings.retentionHint": "File log cũ tự bị xóa.",
    "settings.debug": "Log chi tiết (debug)",
    "settings.debugHint": "Nhiều chi tiết hơn, vẫn đã làm sạch.",
    "settings.danger": "Khu vực nguy hiểm",
    "settings.restore": "Khôi phục mặc định",
    "settings.restoreHint": "Đặt lại toàn bộ cài đặt về giá trị gốc.",
    "settings.reset": "Đặt lại",
    "settings.welcome": "Hiện lại hướng dẫn chào mừng",
    "settings.welcomeHint": "Mở lại hướng dẫn lần đầu vào lần khởi động sau.",
    "settings.showAgain": "Hiện lại",
    "settings.saveChanges": "Lưu thay đổi",
    "settings.saved": "Đã lưu.",
    "settings.error": "Lỗi: {e}",

    "common.cancel": "Hủy",
    "common.gotIt": "Đã hiểu",
    "common.copied": "Đã sao chép",
    "common.copyFail": "Không sao chép được",

    "firstrun.title": "Chào mừng tới Local AI Proxy",
    "firstrun.intro": "LocalAIProxy biến các AI CLI đã cài trên máy thành một API tương thích OpenAI chạy cục bộ.",
    "firstrun.step1": "Khởi động máy chủ cục bộ",
    "firstrun.step2": "Sao chép API URL",
    "firstrun.step3a": "Gọi một model từ tab Tổng quan qua trường",
    "firstrun.step3b": "— mặc định gồm",
    "firstrun.privacy": "Mọi thứ nằm trên máy của bạn. LocalAIProxy không bao giờ quản lý tài khoản đăng nhập của nhà cung cấp.",

    "close.title": "Có request đang chạy",
    "close.body": "Vẫn còn một hoặc nhiều request AI đang xử lý. Đóng bây giờ sẽ hủy chúng.",
    "close.keepRunning": "Chạy tiếp",
    "close.cancelExit": "Hủy và thoát",

    "toast.keyCopied": "Đã sao chép API key",
    "toast.keyGen": "Đã tạo khóa mới",
    "toast.keyGenFail": "Không tạo được khóa",
    "toast.restored": "Đã khôi phục mặc định",
    "toast.showWelcome": "Hướng dẫn chào mừng sẽ hiện vào lần khởi động sau",
    "toast.scanned": "Đã quét nhà cung cấp",
    "toast.copyAlias": "Đã sao chép alias: {alias}",
    "toast.testFailed": "Kiểm tra thất bại: {e}",
    "toast.modelTestOk": "Model {id} OK — {ms}ms",
    "toast.modelTestFail": "Model {id} thất bại: {e}",
    "toast.deleteFail": "Không xóa được: {e}",
    "toast.updateFail": "Không cập nhật được nhà cung cấp: {e}",

    "status.Ready": "Sẵn sàng",
    "status.Disabled": "Đã tắt",
    "status.Not installed": "Chưa cài đặt",
    "status.Backend not installed": "Chưa cài nền tảng",
    "status.Auth required": "Cần đăng nhập",
    "status.Auth unknown": "Chưa rõ trạng thái đăng nhập",

    "activity.ok": "thành công",
    "activity.error": "lỗi",
    "activity.cancelled": "đã hủy",
    "activity.timeout": "hết thời gian",
    "activity.rate_limited": "bị giới hạn tần suất",
    "activity.auth_error": "lỗi xác thực",
    "activity.not_found": "không tìm thấy",
  },
};

const STORAGE_KEY = "lap.lang";
let lang = "en";

function lookup(key, vars) {
  const s = (I18N[lang] && I18N[lang][key]) ?? I18N.en[key];
  if (s === undefined) return null;
  if (!vars) return s;
  return s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}
function t(key, vars) {
  return lookup(key, vars) ?? key;
}
function tStatus(s) {
  return lookup("status." + s) ?? s;
}
function tActivity(s) {
  const v = lookup("activity." + s);
  return v ?? s.replace(/_/g, " ");
}

function syncLangSelects() {
  for (const id of ["lang-select", "lang-setting"]) {
    const el = $(id);
    if (el) el.value = lang;
  }
}

function setLang(next) {
  lang = next === "vi" ? "vi" : "en";
  try { localStorage.setItem(STORAGE_KEY, lang); } catch (e) {}
}

function applyLang() {
  document.documentElement.lang = lang;
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.innerHTML = t(el.dataset.i18n);
  });
  document.querySelectorAll("[data-i18n-placeholder]").forEach((el) => {
    el.placeholder = t(el.dataset.i18nPlaceholder);
  });
  document.querySelectorAll("[data-i18n-title]").forEach((el) => {
    el.title = t(el.dataset.i18nTitle);
  });
  syncLangSelects();
  document.title = "Local AI Proxy";
  if (snapshot) {
    render();
    if (!$("view-providers").hidden) renderProviders();
  }
  renderSettingsForm();
}

/* ---- clipboard --------------------------------------------------------- */

async function copyText(text, okMessage) {
  try {
    if (window.runtime && window.runtime.ClipboardSetText) {
      await window.runtime.ClipboardSetText(text);
    } else {
      await navigator.clipboard.writeText(text);
    }
    toast(okMessage || t("common.copied"));
  } catch (e) {
    toast(t("common.copyFail"));
  }
}

let toastTimer = null;
function toast(msg) {
  const tt = $("toast");
  tt.textContent = msg;
  tt.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { tt.hidden = true; }, Math.min(1600 + msg.length * 15, 4000));
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

  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) setLang(saved);
  } catch (e) {}

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

  applyLang();
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
      if ($("connect-backdrop")) $("connect-backdrop").hidden = true;
    }
  });
}

/* ---- tabs -------------------------------------------------------------- */

function wireTabs() {
  const tabs = ["dashboard", "models", "providers", "activity", "settings"];
  for (const name of tabs) {
    $("tab-" + name).addEventListener("click", () => switchTab(name));
  }
  document.addEventListener("keydown", (e) => {
    const active = document.querySelector(".tab.is-active");
    const idx = ["dashboard", "models", "providers", "activity", "settings"].indexOf(active ? active.id.replace("tab-", "") : "");
    const dir = e.ctrlKey ? (e.key === "Tab" ? (e.shiftKey ? -1 : 1) : 0) : 0;
    if (dir && idx >= 0) {
      e.preventDefault();
      const next = (idx + dir + 5) % 5;
      switchTab(["dashboard", "models", "providers", "activity", "settings"][next]);
    }
  });
}

function switchTab(name) {
  if (name === "providers") renderProviders();
  if (name === "settings") renderSettingsForm();
  if (name === "models") renderModelsFull();
  if (name === "activity") renderActivityFull();
  for (const tt of ["dashboard", "models", "providers", "activity", "settings"]) {
    const tab = $("tab-" + tt);
    if (!tab) continue;
    tab.classList.toggle("is-active", tt === name);
    tab.setAttribute("aria-selected", tt === name ? "true" : "false");
    const view = $("view-" + tt);
    if (view) view.hidden = tt !== name;
  }
}

/* ---- wiring ------------------------------------------------------------ */

function wireButtons() {
  $("lang-select").addEventListener("change", (e) => { setLang(e.target.value); applyLang(); });
  if ($("lang-setting")) $("lang-setting").addEventListener("change", (e) => { setLang(e.target.value); applyLang(); });

  $("btn-copy-url").addEventListener("click", () => copyText(($("api-url").textContent || "").trim(), t("server.copied")));
  $("btn-start").addEventListener("click", async () => {
    $("btn-start").disabled = true;
    try { await window.go.main.App.StartServer(); } catch (e) { toast(t("server.startFail", { e: friendlyErr(e) })); }
    $("btn-start").disabled = false;
  });
  $("btn-stop").addEventListener("click", async () => { await window.go.main.App.StopServer(); });
  $("btn-restart").addEventListener("click", async () => {
    $("btn-restart").disabled = true;
    try { await window.go.main.App.RestartServer(); } catch (e) { toast(t("server.restartFail", { e: friendlyErr(e) })); }
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

  $("btn-copy-key").addEventListener("click", () => copyText(apiKey || "", t("toast.keyCopied")));
  $("btn-gen-key").addEventListener("click", async () => {
    try { apiKey = await window.go.main.App.GenerateAPIKey(); renderSettingsForm(); toast(t("toast.keyGen")); }
    catch (e) { toast(t("toast.keyGenFail")); }
  });

  $("btn-restore").addEventListener("click", async () => {
    await window.go.main.App.RestoreDefaults();
    settings = await window.go.main.App.GetConfig();
    apiKey = await window.go.main.App.GetAPIKey();
    renderSettingsForm();
    toast(t("toast.restored"));
  });
  $("btn-reset-firstrun").addEventListener("click", async () => {
    await window.go.main.App.ResetFirstRun();
    toast(t("toast.showWelcome"));
  });
  $("btn-save-settings").addEventListener("click", saveSettings);

  /* model modal */
  $("btn-add-model").addEventListener("click", () => openModelModal(false));
  if ($("btn-add-model-2")) $("btn-add-model-2").addEventListener("click", () => openModelModal(false));
  if ($("btn-goto-activity")) $("btn-goto-activity").addEventListener("click", () => switchTab("activity"));
  $("btn-model-cancel").addEventListener("click", () => { $("model-backdrop").hidden = true; });
  $("btn-model-save").addEventListener("click", saveModel);
  $("sw-model-enabled").addEventListener("click", () => toggleSwitch($("sw-model-enabled")));
  if ($("in-model-provider")) $("in-model-provider").addEventListener("change", renderModelCaps);

  /* delete confirm */
  $("btn-delete-cancel").addEventListener("click", () => { $("delete-backdrop").hidden = true; deleteTarget = null; });
  $("btn-delete-confirm").addEventListener("click", confirmDelete);

  /* connect helper */
  if ($("btn-connect-close")) $("btn-connect-close").addEventListener("click", () => { $("connect-backdrop").hidden = true; });
  if ($("connect-client")) $("connect-client").addEventListener("change", renderConnectSnippet);
  if ($("btn-copy-connect-url")) $("btn-copy-connect-url").addEventListener("click", () => copyText($("connect-url").textContent.trim(), t("common.copied")));
  if ($("btn-copy-connect-key")) $("btn-copy-connect-key").addEventListener("click", () => copyText($("connect-key").textContent.trim(), t("common.copied")));
  if ($("btn-copy-connect-model")) $("btn-copy-connect-model").addEventListener("click", () => copyText($("connect-model").textContent.trim(), t("common.copied")));
}

function toggleSwitch(sw) {
  const on = sw.getAttribute("aria-checked") === "true";
  sw.setAttribute("aria-checked", on ? "false" : "true");
}

async function refresh() {
  await window.go.main.App.Refresh();
  snapshot = await window.go.main.App.GetSnapshot();
  render();
  toast(t("toast.scanned"));
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
    $("port-error").textContent = t("server.portRange");
    $("port-error").hidden = false;
    return;
  }
  if (port === snapshot.port) return;
  if (await window.go.main.App.PortInUse(port)) {
    $("port-warning").hidden = false;
  }
  try {
    await window.go.main.App.SetPort(port);
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    toast(t("server.portSet", { p: port }));
  } catch (e) {
    const msg = friendlyErr(e);
    $("port-error").textContent = msg + " The proxy is still running on port " + snapshot.port + ". Choose another port.";
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
    const note = !p.installed ? " (" + t("providers.notInstalled") + ")" : (!p.enabled ? " (" + t("settings.disable").toLowerCase() + ")" : "");
    opts += '<option value="' + esc(p.alias) + '">' + esc(p.name) + note + "</option>\n";
  }
  return opts || "";
}

function openModelModal(mode, model) {
  editingId = null;
  $("model-id-error").hidden = true;
  $("model-form-error").hidden = true;

  const isEdit = mode === "edit";
  $("model-modal-title").textContent = isEdit ? t("models.editTitle") : t("models.addTitle");
  $("in-model-provider").innerHTML = providerOptions();
  $("in-model-stream").innerHTML =
    '<option value="native">' + esc(t("models.stream.nativeLabel")) + "</option>" +
    '<option value="disabled">' + esc(t("models.stream.disabledLabel")) + "</option>";

  if (isEdit && model) {
    editingId = model.id;
    $("in-model-id").value = model.id;
    $("in-model-id").disabled = true;
    $("in-model-name").value = model.displayName || "";
    $("in-model-provider").value = model.provider;
    $("in-model-upstream").value = model.upstreamModel || "";
    $("in-model-stream").value = model.streamMode === "disabled" ? "disabled" : "native";
    $("in-model-timeout").value = model.timeoutSeconds || 300;
    $("sw-model-enabled").setAttribute("aria-checked", model.enabled ? "true" : "false");
    if ($("in-model-system")) $("in-model-system").value = "";
    if ($("in-model-temp")) $("in-model-temp").value = "";
    if ($("in-model-maxtok")) $("in-model-maxtok").value = "";
    if ($("in-model-ctx")) $("in-model-ctx").value = "";
  } else {
    $("in-model-id").value = mode === "duplicate" && model ? (model.id + "-copy").slice(0, 63) : "";
    $("in-model-id").disabled = false;
    $("in-model-name").value = model && model.displayName ? model.displayName : "";
    const first = (snapshot.providers || []).find((p) => p.enabled && p.installed);
    $("in-model-provider").value = (model && model.provider) || (first ? first.alias : ((snapshot.providers || [])[0] || {}).alias || "");
    $("in-model-upstream").value = (model && model.upstreamModel) || "";
    $("in-model-stream").value = model && model.streamMode ? model.streamMode : "native";
    $("in-model-timeout").value = model && model.timeoutSeconds ? model.timeoutSeconds : 300;
    $("sw-model-enabled").setAttribute("aria-checked", "true");
  }
  renderModelCaps();

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
    bad = fieldError($("model-id-error"), t("models.idError"));
  }
  if (!timeout || timeout < 1 || timeout > 86400) {
    fieldError($("model-form-error"), t("models.timeoutError"));
    bad = true;
  }
  if (bad) return;

  const tempRaw = $("in-model-temp") ? $("in-model-temp").value.trim() : "";
  const maxRaw = $("in-model-maxtok") ? $("in-model-maxtok").value.trim() : "";
  const ctxRaw = $("in-model-ctx") ? $("in-model-ctx").value.trim() : "";
  const input = {
    id: sId,
    provider: $("in-model-provider").value,
    displayName: $("in-model-name").value.trim(),
    upstreamModel: $("in-model-upstream") ? $("in-model-upstream").value.trim() : "",
    streamMode: $("in-model-stream").value,
    timeoutSeconds: timeout,
    enabled: $("sw-model-enabled").getAttribute("aria-checked") === "true",
    systemPrompt: $("in-model-system") ? $("in-model-system").value.trim() : "",
  };
  if (tempRaw !== "") {
    const tv = parseFloat(tempRaw);
    if (isNaN(tv) || tv < 0 || tv > 2) {
      fieldError($("model-form-error"), "Temperature must be 0–2.");
      return;
    }
    input.temperature = tv;
  }
  if (maxRaw !== "") {
    const mv = parseInt(maxRaw, 10);
    if (!mv || mv < 1) {
      fieldError($("model-form-error"), "Max tokens must be ≥ 1.");
      return;
    }
    input.maxTokens = mv;
  }
  if (ctxRaw !== "") {
    const cv = parseInt(ctxRaw, 10);
    if (!cv || cv < 1) {
      fieldError($("model-form-error"), "Context window must be ≥ 1.");
      return;
    }
    input.contextWindow = cv;
  }
  try {
    await window.go.main.App.SaveModel(input);
    $("model-backdrop").hidden = true;
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    toast(editingId ? t("models.updated", { id: sId }) : t("models.added", { id: sId }));
  } catch (e) {
    fieldError($("model-form-error"), friendlyErr(e));
  }
}

function renderModelCaps() {
  const box = $("model-caps");
  if (!box) return;
  const alias = $("in-model-provider") ? $("in-model-provider").value : "";
  const p = (snapshot.providers || []).find((x) => x.alias === alias);
  const caps = (p && p.capabilities) || {};
  const row = (ok, label) => '<span class="' + (ok ? "cap-ok" : "cap-no") + '">' + (ok ? "✓ " : "○ ") + esc(label) + "</span>";
  box.innerHTML = "<strong>Capabilities</strong> " +
    row(caps.streaming, "Native streaming") +
    row(caps.modelSelection, "Model selection") +
    row(caps.tools, "Tool calling") +
    row(caps.usage, "Usage");
  const streamSel = $("in-model-stream");
  if (streamSel && !caps.streaming && streamSel.value === "native") {
    streamSel.value = "disabled";
  }
}

/* ---- Connect helper ---- */
let connectModel = null;
function openConnect(model) {
  connectModel = model;
  $("connect-sub").textContent = "Model " + model.id + " on " + model.providerName;
  $("connect-url").textContent = snapshot.url;
  $("connect-key").textContent = apiKey || "(no key required)";
  $("connect-model").textContent = model.id;
  renderConnectSnippet();
  $("connect-backdrop").hidden = false;
  $("connect-client").focus();
}
function renderConnectSnippet() {
  if (!connectModel) return;
  const client = $("connect-client").value;
  const url = $("connect-url").textContent.trim();
  const key = apiKey || "YOUR_KEY";
  const mid = connectModel.id;
  let snip = "";
  if (client === "python") snip = 'from openai import OpenAI\nclient = OpenAI(base_url="' + url + '", api_key="' + key + '")\nresp = client.chat.completions.create(model="' + mid + '", messages=[{"role":"user","content":"Hello"}])';
  else if (client === "aider") snip = "export OPENAI_API_BASE=" + url + "\nexport OPENAI_API_KEY=" + key + "\naider --model " + mid;
  else if (client === "cline" || client === "roo") snip = '{\n  "baseUrl": "' + url + '",\n  "apiKey": "' + key + '",\n  "model": "' + mid + '"\n}';
  else if (client === "ainovel") snip = "ainovel-cli --base-url " + url + " --model " + mid;
  else snip = 'Base URL: ' + url + '\nAPI Key: ' + key + '\nModel: ' + mid;
  $("connect-snippet").textContent = snip;
}

async function confirmDelete() {
  if (!deleteTarget) return;
  try {
    await window.go.main.App.DeleteModel(deleteTarget);
    $("delete-backdrop").hidden = true;
    snapshot = await window.go.main.App.GetSnapshot();
    render();
    toast(t("models.deleted", { id: deleteTarget }));
  } catch (e) {
    $("delete-backdrop").hidden = true;
    toast(t("toast.deleteFail", { e: friendlyErr(e) }));
  }
  deleteTarget = null;
}

/* ---- rendering --------------------------------------------------------- */

function render() {
  renderHeader();
  renderDashboard();
  if (!$("view-providers").hidden) renderProviders();
  if (!$("view-models").hidden) renderModelsFull();
  if (!$("view-activity").hidden) renderActivityFull();
}

function renderHeader() {
  const chip = $("server-chip");
  const text = $("server-chip-text");
  const running = snapshot.serverRunning;
  chip.className = "chip " + (running ? "chip-running" : "chip-stopped");
  text.textContent = running ? t("server.chip.running", { p: snapshot.port }) : t("server.chip.stopped");
  $("btn-start").hidden = running;
  $("btn-stop").hidden = !running;
  $("btn-restart").hidden = !running;
  $("api-url").textContent = snapshot.url;
  $("api-url").title = snapshot.url;
  const hint = running && snapshot.requireApiKey;
  $("api-hint").hidden = !hint;
  const active = snapshot.activeRequests || 0;
  let queued = 0;
  for (const p of snapshot.providers || []) queued += p.queueDepth || 0;
  const loadLine = $("load-line");
  if (loadLine) loadLine.textContent = active + " running · " + queued + " queued";
  $("header-port").textContent = snapshot.host + ":" + snapshot.port;
  document.title = "Local AI Proxy — " + (running ? t("server.running") : t("server.stopped"));
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
    body.innerHTML = '<tr class="empty-row"><td colspan="6">' + esc(t("models.empty")) + "</td></tr>";
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
    ? '<span class="pill running"><span class="dot"></span>' + esc(t("models.testing")) + "</span>"
    : statusPill(m.status, m.statusKind);

  tr.appendChild(td(
    '<div class="model-name"><span class="mono strong">' + esc(m.id) + "</span>" +
    (m.displayName ? '<span class="model-disp">' + esc(m.displayName) + "</span>" : "") + "</div>"
  ));
  tr.appendChild(td(
    '<div class="model-backend"><span class="mono">' + esc(m.provider) + "</span>" +
    '<span class="model-disp">' + esc(m.providerName) + (m.upstreamModel ? " · " + esc(m.upstreamModel) : "") + "</span></div>"
  ));
  tr.appendChild(td(streamChip(m)));
  tr.appendChild(td('<span class="mono">' + (m.timeoutSeconds ? m.timeoutSeconds + "s" : t("models.timeout.unlimited")) + "</span>"));
  tr.appendChild(td(pill));
  tr.appendChild(td(
    '<div class="row-actions">' +
      '<button class="btn btn-ghost btn-sm model-action test" data-model="' + esc(m.id) + '" ' +
        (m.enabled && !testing && m.ready ? "" : 'disabled title="not ready"') + ">" +
        (testing ? esc(t("models.testing")) : esc(t("models.test"))) + "</button>" +
      '<button class="btn btn-ghost btn-sm model-action edit" data-model="' + esc(m.id) + '">' + esc(t("models.edit")) + "</button>" +
      '<button class="btn btn-ghost btn-sm model-action dup" data-model="' + esc(m.id) + '">' + esc(t("models.duplicate")) + "</button>" +
      '<button class="btn btn-ghost btn-sm model-action connect" data-model="' + esc(m.id) + '">Connect</button>' +
      '<button class="btn btn-ghost btn-sm model-action copy" data-model="' + esc(m.id) + '" title="Copy Model ID">Copy ID</button>' +
      '<button class="btn btn-danger btn-sm model-action del" data-model="' + esc(m.id) + '">' + esc(t("models.delete")) + "</button>" +
    "</div>"
  ));
  return tr;
}

function renderModelsFull() {
  const body = $("models-body-2");
  if (!body) return;
  const models = snapshot.models || [];
  if (!models.length) {
    body.innerHTML = '<tr class="empty-row"><td colspan="6"><div class="empty-state">No custom models yet.<br>Create a model profile and route it to any available CLI provider.<br><button class="btn btn-primary btn-sm" id="btn-empty-add">Add Model</button></div></td></tr>';
    const b = $("btn-empty-add");
    if (b) b.addEventListener("click", () => openModelModal(false));
    return;
  }
  body.innerHTML = "";
  for (const m of models) body.appendChild(modelRow(m));
}

function renderActivityFull() {
  const body = $("activity-body-full");
  if (!body) return;
  const items = snapshot.activity || [];
  if (!items.length) {
    body.innerHTML = '<tr class="empty-row"><td colspan="8"><div class="empty-state">No requests yet. Send one, or press Test on a model.</div></td></tr>';
    return;
  }
  body.innerHTML = "";
  for (const a of items.slice(0, 100)) {
    const tr = document.createElement("tr");
    tr.appendChild(td('<span class="mono">' + esc(a.time || "") + "</span>"));
    tr.appendChild(td('<span class="mono strong">' + esc(a.model || a.alias || "—") + "</span>"));
    tr.appendChild(td(esc(a.provider || "")));
    tr.appendChild(td(activityStatus(a)));
    tr.appendChild(td(a.stream ? "stream" : "json"));
    tr.appendChild(td(a.durationMs >= 0 ? a.durationMs + "ms" : "—"));
    tr.appendChild(td(a.queueWaitMs != null ? a.queueWaitMs + "ms" : "—"));
    tr.appendChild(td(a.ttftMs != null && a.ttftMs >= 0 ? a.ttftMs + "ms" : "—"));
    body.appendChild(tr);
  }
}

function streamChip(m) {
  const native = m.streamMode === "native";
  return '<span class="stream-chip ' + (native ? "native" : "disabled") + '">' +
    esc(native ? t("models.stream.native") : t("models.stream.disabled")) + "</span>";
}

function renderActivity() {
  const body = $("activity-body");
  const items = snapshot.activity || [];
  if (!items.length) {
    body.innerHTML = '<tr class="empty-row"><td colspan="5">' + esc(t("activity.empty")) + "</td></tr>";
    return;
  }
  body.innerHTML = "";
  for (const a of items.slice(0, 20)) {
    const tr = document.createElement("tr");
    tr.appendChild(td('<span class="mono">' + esc(a.time) + "</span>"));
    tr.appendChild(td(esc(a.provider)));
    tr.appendChild(td((a.model ? esc(a.model) : "") || "—"));
    tr.appendChild(td(activityStatus(a)));
    tr.appendChild(td(a.durationMs >= 0 ? a.durationMs + "ms" : "—"));
    body.appendChild(tr);
  }
}

function activityStatus(a) {
  const label = a.ok ? t("activity.ok") : tActivity(a.status);
  const cls = a.ok ? "ok" : "error";
  return '<span class="pill ' + cls + '"><span class="dot"></span>' + esc(label) + "</span>";
}

function renderProviders() {
  const list = $("provider-list");
  const open = {};
  for (const item of list.querySelectorAll(".provider-item.open")) open[item.id] = true;

  const ps = snapshot.providers || [];
  if (!ps.length) {
    list.innerHTML =
      '<div class="panel"><div class="panel-body"><span class="hint">' + esc(t("providers.empty")) + "</span></div></div>";
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
        (canTest ? "" : " disabled") + ">" + (testState ? esc(t("models.testing")) : esc(t("models.test"))) + "</button>" +
      '<button class="btn btn-ghost btn-sm copy-alias" data-alias="' + esc(p.alias) + '">' + ICONS.copy + " " + esc(t("providers.copyAlias")) + "</button>" +
      '<button class="chevron" aria-label="' + esc(p.name) + '" data-toggle="' + esc(p.alias) + '">' + ICONS.chevron + "</button>" +
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
  if (!p.installed) return t("providers.notInstalled");
  return (p.version || "version unknown") + " · " + authLabel(p.auth);
}

function authLabel(a) {
  if (a === "detected") return t("providers.auth.detected");
  if (a === "required") return t("providers.auth.required");
  return t("providers.auth.unknown");
}

function detailBody(p) {
  const wrap = document.createElement("div");

  const defs = document.createElement("dl");
  defs.className = "defs";
  const caps = p.capabilities || {};
  const capsLine = ["streaming", "modelSelection", "tools", "usage", "vision", "sessions"]
    .filter((k) => caps[k] || k === "streaming" || k === "modelSelection")
    .map((k) => (caps[k] ? "✓ " : "○ ") + k).join(" · ");
  defs.innerHTML =
    def(t("providers.executable"), '<span class="code" title="' + esc(p.executable || "") + '">' + esc(truncMid(p.executable || "—", 60)) + "</span>") +
    def(t("providers.version"), esc(p.version || "—")) +
    def(t("providers.authentication"), authLabel(p.auth)) +
    def(t("providers.state"), tStatus(p.status)) +
    def("Capabilities", esc(capsLine || "—")) +
    def("Concurrency", esc(String(p.concurrency)) + " · Queue " + esc(String(p.maxQueue))) +
    def("Load", esc(String(p.queueDepth || 0)) + " queued");

  const adv = document.createElement("div");
  adv.className = "pdetails-adv";
  adv.innerHTML =
    advNum(t("providers.concurrency"), p.concurrency) +
    advNum(t("providers.maxQueued"), p.maxQueue) +
    advNum(t("providers.queueTimeout"), p.queueTimeoutSec) +
    advNum(t("providers.execTimeout"), p.execTimeoutSec === 0 ? t("providers.unlimited") : p.execTimeoutSec);

  wrap.appendChild(defs);

  if (!p.installed) {
    const mb = document.createElement("div");
    mb.className = "missing-box";
    mb.innerHTML =
      "<span><strong>" + esc(t("providers.missingTitle", { name: p.name })) + "</strong><br>" + esc(t("providers.missingBody")) + "</span>" +
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
    (p.enabled ? esc(t("providers.disable")) : esc(t("providers.enable"))) + "</button>";
  wrap.appendChild(enabledRow);

  wireAfterRender(wrap, p);
  return wrap;
}

function wireAfterRender(wrap, p) {
  const testBtn = wrap.querySelector(".test-btn");
  if (testBtn) testBtn.addEventListener("click", () => runTest(p.alias));
  const copyBtn = wrap.querySelector(".copy-alias");
  if (copyBtn) copyBtn.addEventListener("click", () => copyText(p.alias, t("toast.copyAlias", { alias: p.alias })));
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
    toast(p.enabled ? t("providers.toggledOff", { alias }) : t("providers.toggledOn", { alias }));
  } catch (e) {
    toast(t("toast.updateFail", { e: friendlyErr(e) }));
  }
}

function lastTestBlock(test) {
  const cls = test.passed ? "ok" : "fail";
  const headline = test.passed
    ? t("providers.testPassed", { ms: test.latencyMs })
    : t("providers.testFailed", { s: (test.message || "error").replace(/_/g, " ") });
  return '<div class="test-result ' + cls + '">' +
    "<strong>" + esc(headline) + "</strong>" +
    (test.passed ? '<div class="detail">' + esc(t("providers.response")) + ' "' + esc(truncMid(test.response || "", 120)) + '"</div>' : "") +
    (!test.passed ? '<div class="detail">' + esc(test.detail || test.message || "") + "</div>" : "") +
    "</div>";
}

function def(k, v) {
  return '<div class="def"><dt>' + esc(k) + "</dt><dd>" + v + "</dd></div>";
}
function advNum(k, v) {
  return '<div class="adv-field"><label>' + esc(k) + '</label><code class="host-val">' + esc(String(v)) + "</code></div>";
}

function statusPill(status, kind, testing) {
  if (testing) return '<span class="pill running"><span class="dot"></span>' + esc(t("models.testing")) + "</span>";
  const k = kind || "idle";
  return '<span class="pill ' + esc(k) + '"><span class="dot"></span>' + esc(tStatus(status)) + "</span>";
}

/* event delegation for dynamic content */
document.addEventListener("click", (e) => {
  const testBtn = e.target.closest(".test-btn");
  if (testBtn) { runTest(testBtn.dataset.alias); return; }

  const copyAlias = e.target.closest(".copy-alias");
  if (copyAlias) { copyText(copyAlias.dataset.alias, t("toast.copyAlias", { alias: copyAlias.dataset.alias })); return; }

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
    } else if (action.classList.contains("connect")) {
      const m = (snapshot.models || []).find((x) => x.id === id);
      if (m) openConnect(m);
    } else if (action.classList.contains("copy")) {
      copyText(id, t("common.copied"));
    } else if (action.classList.contains("del")) {
      const m = (snapshot.models || []).find((x) => x.id === id);
      deleteTarget = id;
      $("delete-model-name").textContent = t("models.deleteMsg", {
        id: m ? m.id : id,
        disp: m && m.displayName ? " (" + m.displayName + ")" : "",
      });
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
    toast(t("toast.testFailed", { e: friendlyErr(e) }));
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
      ? t("toast.modelTestOk", { id, ms: res.latencyMs })
      : t("toast.modelTestFail", { id, e: friendlyErr(res && res.message ? res.message : "request failed") }));
  } catch (e) {
    toast(t("toast.testFailed", { e: friendlyErr(e) }));
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
  if ($("in-global-conc")) $("in-global-conc").value = settings.globalConcurrency || 1;
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
  syncLangSelects();
}

function advCard(alias, p) {
  const card = document.createElement("div");
  card.className = "adv-card";
  card.innerHTML =
    '<div class="aliastitle"><span class="palias">' + esc(alias) + "</span><span>" + cap(alias) + "</span></div>" +
    numField(t("providers.concurrency"), "concurrency-" + alias, p.concurrency || 1, 1, 64) +
    numField(t("providers.maxQueued"), "maxQueue-" + alias, p.maxQueue ?? 10, 0, 10000) +
    numField(t("providers.queueTimeout"), "queueTimeoutSec-" + alias, p.queueTimeoutSec ?? 120, 0, 3600) +
    numField(t("providers.execTimeout"), "execTimeoutSec-" + alias, p.execTimeoutSec ?? 0, 0, 86400);
  return card;
}

function numField(labelText, id, value, min, max) {
  return '<div class="adv-num"><label for="' + id + '">' + labelText + '</label>' +
    '<input type="number" class="input" id="' + id + '" min="' + min + '" max="' + max + '" value="' + value + '" aria-label="' + labelText + '"></div>';
}

function cap(s) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

async function saveSettings() {
  const port = parseInt($("in-port").value, 10);
  if (!port || port < 1 || port > 65535) {
    $("settings-status").textContent = t("server.portRange");
    return;
  }
  const globalConcurrency = $("in-global-conc") ? clampInt($("in-global-conc").value, 1, 64, 1) : 1;
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
      globalConcurrency,
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
    $("settings-status").textContent = t("settings.saved");
    autoClearStatus();
  } catch (e) {
    $("settings-status").textContent = t("settings.error", { e: friendlyErr(e) });
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