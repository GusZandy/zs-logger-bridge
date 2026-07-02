// No build step, no framework: this talks directly to the Go backend via
// the bindings Wails injects at runtime (window.go.main.App.*) and the
// event bus (window.runtime.Events*). See app.go for the bound methods.

const els = {
  serverUrl: document.getElementById("serverUrl"),
  logsheetId: document.getElementById("logsheetId"),
  token: document.getElementById("token"),
  n1mmEnabled: document.getElementById("n1mmEnabled"),
  n1mmPort: document.getElementById("n1mmPort"),
  jtdxEnabled: document.getElementById("jtdxEnabled"),
  jtdxPort: document.getElementById("jtdxPort"),
  saveBtn: document.getElementById("saveBtn"),
  startBtn: document.getElementById("startBtn"),
  stopBtn: document.getElementById("stopBtn"),
  quitBtn: document.getElementById("quitBtn"),
  statusDot: document.getElementById("status-dot"),
  statusText: document.getElementById("status-text"),
  activity: document.getElementById("activity"),
};

function formToConfig() {
  return {
    server_url: els.serverUrl.value.trim(),
    logsheet_id: els.logsheetId.value.trim(),
    token: els.token.value.trim(),
    n1mm_enabled: els.n1mmEnabled.checked,
    n1mm_port: parseInt(els.n1mmPort.value, 10) || 12060,
    jtdx_enabled: els.jtdxEnabled.checked,
    jtdx_port: parseInt(els.jtdxPort.value, 10) || 2237,
  };
}

function configToForm(cfg) {
  els.serverUrl.value = cfg.server_url || "";
  els.logsheetId.value = cfg.logsheet_id || "";
  els.token.value = cfg.token || "";
  els.n1mmEnabled.checked = !!cfg.n1mm_enabled;
  els.n1mmPort.value = cfg.n1mm_port || 12060;
  els.jtdxEnabled.checked = !!cfg.jtdx_enabled;
  els.jtdxPort.value = cfg.jtdx_port || 2237;
}

function setStatus(running) {
  els.statusDot.classList.toggle("on", running);
  els.statusText.textContent = running ? "Running" : "Stopped";
  els.startBtn.disabled = running;
  els.stopBtn.disabled = !running;
}

function appendActivity(entry) {
  const row = document.createElement("div");
  row.className = "entry";
  row.innerHTML = `<span class="time">${entry.time}</span><span class="${entry.level}">${escapeHtml(entry.message)}</span>`;
  els.activity.appendChild(row);
  els.activity.scrollTop = els.activity.scrollHeight;
}

function escapeHtml(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

async function init() {
  const app = window.go.main.App;

  const cfg = await app.GetConfig();
  configToForm(cfg);

  const running = await app.IsRunning();
  setStatus(running);

  const history = await app.GetActivity();
  history.forEach(appendActivity);

  els.saveBtn.addEventListener("click", async () => {
    await app.SaveConfig(formToConfig());
  });

  els.startBtn.addEventListener("click", async () => {
    await app.SaveConfig(formToConfig());
    try {
      await app.StartBridge();
    } catch (e) {
      appendActivity({ time: new Date().toLocaleTimeString(), level: "error", message: String(e) });
    }
  });

  els.stopBtn.addEventListener("click", async () => {
    await app.StopBridge();
  });

  els.quitBtn.addEventListener("click", async () => {
    await app.Quit();
  });

  window.runtime.EventsOn("activity", appendActivity);
  window.runtime.EventsOn("status", setStatus);
}

window.addEventListener("DOMContentLoaded", init);
