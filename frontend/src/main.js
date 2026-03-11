import "./style.css";
import "./app.css";

import { EventsOn } from "../wailsjs/runtime/runtime";

const appRoot = document.querySelector("#app");

const state = {
  backends: [],
  compressions: [],
  defaultPorts: {},
  backendHints: {},
  running: false,
  zipBusy: false,
  rowsWritten: 0,
  status: "待命",
  logs: [],
  notice: null,
  exportConflict: null,
  generatedFiles: [],
  selectedFiles: [],
  selectionInitialized: false,
  zipEnabled: false,
  zipPassword: "",
  visibility: {
    password: false,
    maxcomputeAccessKey: false,
    zipPassword: false,
  },
  form: {
    backend: "oracle",
    outputPath: "",
    table: "",
    batchSize: 5000,
    compression: "snappy",
    schema: "",
    host: "",
    port: 1521,
    username: "",
    password: "",
    database: "",
    maxcomputeEndpoint: "",
    maxcomputeProject: "",
    maxcomputeAccessId: "",
    maxcomputeAccessKey: "",
    partitionSpec: "",
  },
};

let noticeTimer = null;
let lastRenderedLogCount = 0;
let stickLogToBottom = true;

function getAppBinding() {
  const appBinding = window.go?.main?.App || window.go?.core?.App;
  if (!appBinding) {
    throw new Error("Wails App binding is unavailable");
  }
  return appBinding;
}

function GetConfig() {
  return getAppBinding().GetConfig();
}

function CheckExportOutput(path) {
  return getAppBinding().CheckExportOutput(path);
}

function GetGeneratedFiles() {
  return getAppBinding().GetGeneratedFiles();
}

function StartTask(kind, request) {
  return getAppBinding().StartTask(kind, request);
}

function CreateZipArchive(request) {
  return getAppBinding().CreateZipArchive(request);
}

function render() {
  appRoot.innerHTML = `
    <div class="page-shell">
      <section class="hero-card">
        <div class="hero-bar">
          <div>
            <h1>Parquet Export Studio</h1>
            <p class="hero-copy">从 Oracle / MySQL / PostgreSQL / MaxCompute 导出单表到本地 Parquet</p>
            <p class="hero-hint">${backendHint()}</p>
          </div>
          <div class="hero-metrics">
            <div class="hero-status">${escapeHtml(state.status)}</div>
            <div class="hero-rows">已写入 ${numberWithCommas(state.rowsWritten)} 行</div>
          </div>
        </div>

        <div class="panel-grid">
          <section class="panel panel-left">
            <h2>连接配置</h2>
            ${renderFieldShell("数据源", `<select id="backend">${renderBackendOptions()}</select>`, "field-select")}
            ${renderConnectionFields()}
            ${renderObjectFields()}
          </section>

          <div class="panel-stack">
            <section class="panel">
              <h2>导出参数</h2>
              ${renderFieldShell("输出 Parquet 路径", `<input id="outputPath" value="${escapeAttr(state.form.outputPath)}" />`)}
              <div class="field-row">
                <div class="grow">
                  ${renderFieldShell("批次大小", `<input id="batchSize" type="number" min="1" step="1000" value="${escapeAttr(String(state.form.batchSize))}" />`)}
                </div>
                <div class="grow">
                  ${renderFieldShell("压缩", `<select id="compression">${renderCompressionOptions()}</select>`, "field-select")}
                </div>
              </div>
              <p class="panel-note">当数据量大时，可以适当调整批次大小。</p>
              <div class="button-row">
                <button id="testButton" class="button button-secondary" ${state.running ? "disabled" : ""}>测试连接</button>
                <button id="exportButton" class="button button-primary" ${state.running ? "disabled" : ""}>开始导出</button>
              </div>
            </section>

            <section class="panel">
              <h2>运行日志</h2>
              <textarea id="logView" readonly placeholder="日志会显示在这里">${escapeHtml(state.logs.join("\n"))}</textarea>
            </section>

            <section class="panel">
              <h2>本次生成的 Parquet</h2>
              ${renderGeneratedFilesPanel()}
            </section>
          </div>
        </div>
      </section>
      ${renderNotice()}
      ${renderExportConflictDialog()}
    </div>
  `;

  bindInputs();
  syncLogScroll();
}

function bindInputs() {
  document.querySelector("#backend").addEventListener("change", (event) => {
    const backend = event.target.value;
    state.form.backend = backend;
    state.form.port = state.defaultPorts[backend] || 0;
    render();
  });

  [
    ["host", "host"],
    ["port", "port"],
    ["username", "username"],
    ["password", "password"],
    ["database", "database"],
    ["schema", "schema"],
    ["table", "table"],
    ["maxcomputeEndpoint", "maxcomputeEndpoint"],
    ["maxcomputeProject", "maxcomputeProject"],
    ["maxcomputeAccessId", "maxcomputeAccessId"],
    ["maxcomputeAccessKey", "maxcomputeAccessKey"],
    ["partitionSpec", "partitionSpec"],
    ["outputPath", "outputPath"],
    ["batchSize", "batchSize"],
    ["compression", "compression"],
  ].forEach(([id, key]) => {
    const element = document.querySelector(`#${id}`);
    if (!element) {
      return;
    }
    element.addEventListener("input", (event) => {
      const value = event.target.value;
      if (key === "port" || key === "batchSize") {
        state.form[key] = value === "" ? 0 : Number(value);
      } else if (key === "table") {
        state.form.table = value;
        state.form.outputPath = replaceOutputFilename(state.form.outputPath, value);
        const outputPathInput = document.querySelector("#outputPath");
        if (outputPathInput) {
          outputPathInput.value = state.form.outputPath;
        }
      } else {
        state.form[key] = value;
      }
    });
  });

  document.querySelector("#testButton").addEventListener("click", () => startTask("test"));
  document.querySelector("#exportButton").addEventListener("click", () => startExportTask());
  bindVisibilityToggle("password");
  bindVisibilityToggle("maxcomputeAccessKey");
  bindVisibilityToggle("zipPassword");
  bindGeneratedFilesActions();
  bindExportConflictActions();
  bindLogScrollTracking();
}

async function startTask(kind) {
  try {
    await StartTask(kind, sanitizeRequest());
  } catch (error) {
    appendLog(`ERROR: ${error}`);
    state.status = "执行失败";
    showNotice(String(error), "error");
    render();
  }
}

async function startExportTask() {
  const request = sanitizeRequest();

  try {
    const check = await CheckExportOutput(request.outputPath);
    if (check.exists) {
      state.exportConflict = {
        request,
        resolvedPath: check.resolvedPath,
        suggestedPath: check.suggestedPath,
      };
      render();
      return;
    }

    state.form.outputPath = check.resolvedPath;
    await StartTask("export", { ...request, outputPath: check.resolvedPath, conflictPolicy: "overwrite" });
  } catch (error) {
    appendLog(`ERROR: ${error}`);
    state.status = "执行失败";
    showNotice(String(error), "error");
    render();
  }
}

function sanitizeRequest() {
  return {
    ...state.form,
    batchSize: Number(state.form.batchSize) || 1,
    port: Number(state.form.port) || 0,
  };
}

function installEventBridge() {
  EventsOn("task:event", (event) => {
    switch (event.type) {
      case "running":
        state.running = event.message === "true";
        if (state.running) {
          state.status = event.kind === "export" ? "导出中" : "测试中";
          if (event.kind === "export") {
            state.rowsWritten = 0;
          }
          clearNotice();
        } else if (state.status === "导出中" || state.status === "测试中") {
          state.status = "待命";
        }
        break;
      case "log":
        appendLog(event.message);
        break;
      case "success":
        if (event.kind === "export") {
          state.status = "导出完成";
          state.rowsWritten = event.rowsWritten;
          appendLog(`导出成功: ${event.outputPath}`);
          showNotice(`导出成功: ${event.outputPath}`, "success");
          refreshGeneratedFiles();
        } else {
          state.status = "连接成功";
          appendLog("连接成功");
          showNotice("连接成功", "success");
        }
        break;
      case "error":
        state.status = "执行失败";
        appendLog(`ERROR: ${event.error}`);
        showNotice(event.error, "error");
        break;
      default:
        appendLog(`WARN: 未知事件 ${event.type}`);
        break;
    }
    render();
  });
}

function backendHint() {
  return state.backendHints[state.form.backend] || "";
}

function isMaxCompute() {
  return state.form.backend === "maxcompute";
}

function renderConnectionFields() {
  if (isMaxCompute()) {
    return `
      ${renderFieldShell("Endpoint", `<input id="maxcomputeEndpoint" value="${escapeAttr(state.form.maxcomputeEndpoint)}" placeholder="https://service.cn-hangzhou.maxcompute.aliyun.com/api" />`)}
      ${renderFieldShell("Project", `<input id="maxcomputeProject" value="${escapeAttr(state.form.maxcomputeProject)}" />`)}
      <div class="field-row">
        <div class="grow">
          ${renderFieldShell("Access ID", `<input id="maxcomputeAccessId" value="${escapeAttr(state.form.maxcomputeAccessId)}" />`)}
        </div>
        <div class="grow">
          ${renderFieldShell("Access Key", `<input id="maxcomputeAccessKey" type="${fieldInputType("maxcomputeAccessKey")}" value="${escapeAttr(state.form.maxcomputeAccessKey)}" />`, "field-password", "maxcomputeAccessKey")}
        </div>
      </div>
    `;
  }

  return `
    ${renderFieldShell("Host", `<input id="host" value="${escapeAttr(state.form.host)}" />`)}
    ${renderFieldShell("Port", `<input id="port" value="${escapeAttr(String(state.form.port || ""))}" />`)}
    <div class="field-row">
      <div class="grow">
        ${renderFieldShell("Username", `<input id="username" value="${escapeAttr(state.form.username)}" />`)}
      </div>
      <div class="grow">
        ${renderFieldShell("Password", `<input id="password" type="${fieldInputType("password")}" value="${escapeAttr(state.form.password)}" />`, "field-password", "password")}
      </div>
    </div>
    ${renderFieldShell("Database / Service Name", `<input id="database" value="${escapeAttr(state.form.database)}" />`)}
  `;
}

function renderObjectFields() {
  if (isMaxCompute()) {
    return `
      <div class="field-row">
        <div class="grow">
          ${renderFieldShell("Schema (optional)", `<input id="schema" value="${escapeAttr(state.form.schema)}" />`)}
        </div>
        <div class="grow">
          ${renderFieldShell("Table", `<input id="table" value="${escapeAttr(state.form.table)}" />`)}
        </div>
      </div>
      ${renderFieldShell("Partition Spec (optional)", `<input id="partitionSpec" value="${escapeAttr(state.form.partitionSpec)}" placeholder="ds='2026-03-07', region='cn'" />`)}
    `;
  }

  return `
    <div class="field-row">
      <div class="grow">
        ${renderFieldShell("Schema (optional)", `<input id="schema" value="${escapeAttr(state.form.schema)}" />`)}
      </div>
      <div class="grow">
        ${renderFieldShell("Table", `<input id="table" value="${escapeAttr(state.form.table)}" />`)}
      </div>
    </div>
  `;
}

function renderBackendOptions() {
  return state.backends
    .map(
      (option) =>
        `<option value="${option.value}" ${option.value === state.form.backend ? "selected" : ""}>${option.label}</option>`,
    )
    .join("");
}

function renderCompressionOptions() {
  return state.compressions
    .map(
      (option) => `<option value="${option}" ${option === state.form.compression ? "selected" : ""}>${option}</option>`,
    )
    .join("");
}

function renderNotice() {
  if (!state.notice?.message) {
    return "";
  }

  const icon = state.notice.tone === "success" ? "✓" : "⚠";
  return `
    <div class="notice-banner notice-${state.notice.tone}" role="status" aria-live="polite">
      <span class="notice-icon" aria-hidden="true">${icon}</span>
      <span class="notice-message">${escapeHtml(state.notice.message)}</span>
    </div>
  `;
}

function renderExportConflictDialog() {
  if (!state.exportConflict) {
    return "";
  }

  return `
    <div class="dialog-backdrop" data-export-conflict-dismiss="true">
      <div class="dialog-card" role="dialog" aria-modal="true" aria-labelledby="exportConflictTitle">
        <h3 id="exportConflictTitle">输出文件已存在</h3>
        <p class="dialog-copy">目标路径已经存在同名文件。请选择直接覆盖，或自动改名后再导出。</p>
        <div class="dialog-paths">
          <div class="dialog-path-row">
            <span>当前目标</span>
            <code>${escapeHtml(state.exportConflict.resolvedPath)}</code>
          </div>
          <div class="dialog-path-row">
            <span>自动改名</span>
            <code>${escapeHtml(state.exportConflict.suggestedPath)}</code>
          </div>
        </div>
        <div class="dialog-actions">
          <button type="button" class="button button-ghost" data-export-conflict-action="cancel">取消</button>
          <button type="button" class="button button-secondary" data-export-conflict-action="rename">自动改名</button>
          <button type="button" class="button button-primary" data-export-conflict-action="overwrite">覆盖原文件</button>
        </div>
      </div>
    </div>
  `;
}

function renderGeneratedFilesPanel() {
  const allSelected = state.generatedFiles.length > 0 && state.selectedFiles.length === state.generatedFiles.length;
  const selectedCount = state.selectedFiles.length;

  return `
    <div class="generated-toolbar">
      <label class="list-checkbox master-checkbox">
        <input id="selectAllGenerated" type="checkbox" ${allSelected ? "checked" : ""} ${state.generatedFiles.length === 0 ? "disabled" : ""} />
        <span>全选文件</span>
      </label>
      <span class="generated-meta">已选 ${selectedCount} / ${state.generatedFiles.length}</span>
    </div>
    <div class="generated-list ${state.generatedFiles.length === 0 ? "is-empty" : ""}">
      ${state.generatedFiles.length === 0 ? '<p class="empty-copy">当前会话还没有生成 parquet 文件。</p>' : state.generatedFiles.map((file) => renderGeneratedFileItem(file)).join("")}
    </div>
    <div class="zip-controls">
      <div class="zip-surface">
      <label class="list-checkbox">
        <input id="zipEnabled" type="checkbox" ${state.zipEnabled ? "checked" : ""} ${state.generatedFiles.length === 0 ? "disabled" : ""} />
        <span>ZIP 压缩选中文件</span>
      </label>
      ${state.zipEnabled ? renderFieldShell("加密密码（可选）", `<input id="zipPassword" type="${fieldInputType("zipPassword")}" value="${escapeAttr(state.zipPassword)}" placeholder="留空则生成不加密 ZIP" />`, "field-password compact-field", "zipPassword") : ""}
      <div class="zip-actions">
        <button id="createZipButton" class="button button-secondary" ${!state.zipEnabled || selectedCount === 0 || state.zipBusy ? "disabled" : ""}>${state.zipBusy ? "正在打包…" : "创建 ZIP"}</button>
        <p class="zip-note">ZIP 将保存到首个选中文件所在目录。</p>
      </div>
      </div>
    </div>
  `;
}

function renderGeneratedFileItem(file) {
  const checked = state.selectedFiles.includes(file.path);
  return `
    <label class="generated-item ${checked ? "is-selected" : ""}">
      <span class="list-checkbox">
        <input class="generated-file-checkbox" data-file-path="${escapeAttr(file.path)}" type="checkbox" ${checked ? "checked" : ""} />
        <span class="generated-file-copy">
          <strong>${escapeHtml(file.name)}</strong>
        </span>
      </span>
      <span class="generated-file-meta">${formatFileSize(file.size)}</span>
    </label>
  `;
}

function renderFieldShell(label, control, extraClass = "", visibilityKey = "") {
  const className = ["field", extraClass].filter(Boolean).join(" ");
  const toggle = visibilityKey ? renderVisibilityToggle(visibilityKey) : "";
  return `
    <label class="${className}">
      <span>${escapeHtml(label)}</span>
      ${control}
      ${toggle}
    </label>
  `;
}

function renderVisibilityToggle(key) {
  const visible = Boolean(state.visibility[key]);
  const buttonLabel = visible ? "隐藏" : "显示";
  const icon = visible ? visibleEyeIcon() : hiddenEyeIcon();
  return `
    <button
      type="button"
      class="field-visibility-toggle"
      data-visibility-key="${escapeAttr(key)}"
      aria-label="${buttonLabel}"
      aria-pressed="${visible ? "true" : "false"}"
      title="${buttonLabel}"
    >${icon}</button>
  `;
}

function bindVisibilityToggle(key) {
  const button = document.querySelector(`[data-visibility-key="${key}"]`);
  if (!button) {
    return;
  }

  button.addEventListener("click", () => {
    state.visibility[key] = !state.visibility[key];
    render();
    const input = document.querySelector(`#${key}`);
    if (input) {
      input.focus();
      const end = input.value.length;
      input.setSelectionRange?.(end, end);
    }
  });
}

function bindGeneratedFilesActions() {
  const selectAll = document.querySelector("#selectAllGenerated");
  if (selectAll) {
    selectAll.addEventListener("change", (event) => {
      state.selectedFiles = event.target.checked ? state.generatedFiles.map((file) => file.path) : [];
      render();
    });
  }

  document.querySelectorAll(".generated-file-checkbox").forEach((element) => {
    element.addEventListener("change", (event) => {
      const path = event.target.dataset.filePath;
      if (!path) {
        return;
      }

      if (event.target.checked) {
        if (!state.selectedFiles.includes(path)) {
          state.selectedFiles = [...state.selectedFiles, path];
        }
      } else {
        state.selectedFiles = state.selectedFiles.filter((item) => item !== path);
      }
      render();
    });
  });

  const zipEnabled = document.querySelector("#zipEnabled");
  if (zipEnabled) {
    zipEnabled.addEventListener("change", (event) => {
      state.zipEnabled = event.target.checked;
      if (!state.zipEnabled) {
        state.zipPassword = "";
        state.visibility.zipPassword = false;
      }
      render();
    });
  }

  const zipPassword = document.querySelector("#zipPassword");
  if (zipPassword) {
    zipPassword.addEventListener("input", (event) => {
      state.zipPassword = event.target.value;
    });
  }

  const createZipButton = document.querySelector("#createZipButton");
  if (createZipButton) {
    createZipButton.addEventListener("click", () => createZipArchiveFromSelection());
  }
}

function bindExportConflictActions() {
  const backdrop = document.querySelector('[data-export-conflict-dismiss="true"]');
  if (backdrop) {
    backdrop.addEventListener("click", (event) => {
      if (event.target === backdrop) {
        state.exportConflict = null;
        render();
      }
    });
  }

  document.querySelectorAll("[data-export-conflict-action]").forEach((element) => {
    element.addEventListener("click", async (event) => {
      const action = event.currentTarget.dataset.exportConflictAction;
      if (action === "cancel") {
        state.exportConflict = null;
        render();
        return;
      }

      await confirmExportConflict(action);
    });
  });
}

async function confirmExportConflict(action) {
  if (!state.exportConflict) {
    return;
  }

  const { request, resolvedPath, suggestedPath } = state.exportConflict;
  const overwrite = action === "overwrite";
  const nextPath = overwrite ? resolvedPath : suggestedPath;

  state.exportConflict = null;
  state.form.outputPath = nextPath;
  render();

  try {
    await StartTask("export", {
      ...request,
      outputPath: nextPath,
      conflictPolicy: overwrite ? "overwrite" : "rename",
    });
  } catch (error) {
    appendLog(`ERROR: ${error}`);
    state.status = "执行失败";
    showNotice(String(error), "error");
    render();
  }
}

function fieldInputType(key) {
  return state.visibility[key] ? "text" : "password";
}

function hiddenEyeIcon() {
  return `
    <svg viewBox="0 0 20 20" aria-hidden="true">
      <path d="M2.4 10C3.86 6.95 6.66 5 10 5C13.34 5 16.14 6.95 17.6 10C16.14 13.05 13.34 15 10 15C6.66 15 3.86 13.05 2.4 10Z" />
      <circle cx="10" cy="10" r="2.4" />
      <path d="M4 4L16 16" />
    </svg>
  `;
}

function visibleEyeIcon() {
  return `
    <svg viewBox="0 0 20 20" aria-hidden="true">
      <path d="M2.4 10C3.86 6.95 6.66 5 10 5C13.34 5 16.14 6.95 17.6 10C16.14 13.05 13.34 15 10 15C6.66 15 3.86 13.05 2.4 10Z" />
      <circle cx="10" cy="10" r="2.4" />
    </svg>
  `;
}

function appendLog(message) {
  const timestamp = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  state.logs.push(`[${timestamp}] ${message}`);
  state.logs = state.logs.slice(-300);
}

function syncLogScroll() {
  const logView = document.querySelector("#logView");
  if (!logView) {
    lastRenderedLogCount = state.logs.length;
    return;
  }

  if (state.logs.length > lastRenderedLogCount || stickLogToBottom) {
    requestAnimationFrame(() => {
      logView.scrollTop = logView.scrollHeight;
    });
  }

  lastRenderedLogCount = state.logs.length;
}

function bindLogScrollTracking() {
  const logView = document.querySelector("#logView");
  if (!logView) {
    return;
  }

  logView.addEventListener("scroll", () => {
    const distanceToBottom = logView.scrollHeight - logView.scrollTop - logView.clientHeight;
    stickLogToBottom = distanceToBottom < 12;
  });
}

function showNotice(message, tone = "error") {
  state.notice = {
    message: String(message || ""),
    tone,
  };

  if (noticeTimer) {
    window.clearTimeout(noticeTimer);
  }

  noticeTimer = window.setTimeout(() => {
    state.notice = null;
    noticeTimer = null;
    render();
  }, 4800);
}

function clearNotice() {
  state.notice = null;
  if (noticeTimer) {
    window.clearTimeout(noticeTimer);
    noticeTimer = null;
  }
}

function replaceOutputFilename(currentPath, tableName) {
  const trimmedTable = String(tableName || "").trim();
  if (!trimmedTable) {
    return currentPath;
  }

  const normalizedPath = String(currentPath || "").trim();
  const separatorIndex = Math.max(normalizedPath.lastIndexOf("/"), normalizedPath.lastIndexOf("\\"));
  const directory = separatorIndex >= 0 ? normalizedPath.slice(0, separatorIndex + 1) : "";
  const currentFile = separatorIndex >= 0 ? normalizedPath.slice(separatorIndex + 1) : normalizedPath;
  const extensionIndex = currentFile.lastIndexOf(".");
  const extension = extensionIndex > 0 ? currentFile.slice(extensionIndex) : ".parquet";

  return `${directory}${trimmedTable}${extension}`;
}

function numberWithCommas(value) {
  return new Intl.NumberFormat("zh-CN").format(value || 0);
}

function formatFileSize(value) {
  const size = Number(value) || 0;
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  if (size < 1024 * 1024 * 1024) {
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function escapeAttr(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function escapeHtml(value) {
  return String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

async function waitForWailsBridge(timeoutMs = 4000) {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    if (window.runtime && (window.go?.main?.App || window.go?.core?.App)) {
      return;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 16));
  }

  throw new Error("Wails runtime bridge did not initialise in time");
}

async function bootstrap() {
  await waitForWailsBridge();
  const config = await GetConfig();
  state.backends = config.backends;
  state.compressions = config.compressions;
  state.defaultPorts = config.defaultPorts;
  state.backendHints = config.backendHints;
  state.form = { ...state.form, ...config.defaultState };
  await refreshGeneratedFiles();
  installEventBridge();
  render();
}

async function refreshGeneratedFiles() {
  const files = await GetGeneratedFiles();
  const previousSelection = new Set(state.selectedFiles);
  const previousPaths = new Set(state.generatedFiles.map((file) => file.path));

  state.generatedFiles = files;
  state.selectedFiles = files
    .filter((file) => !state.selectionInitialized || previousSelection.has(file.path) || !previousPaths.has(file.path))
    .map((file) => file.path);
  state.selectionInitialized = true;
  render();
}

async function createZipArchiveFromSelection() {
  if (!state.zipEnabled || state.selectedFiles.length === 0 || state.zipBusy) {
    return;
  }

  state.zipBusy = true;
  render();

  try {
    const result = await CreateZipArchive({
      files: state.selectedFiles,
      password: state.zipPassword,
    });
    appendLog(`ZIP 创建成功: ${result.outputPath}`);
    showNotice(`ZIP 创建成功: ${result.outputPath}`, "success");
  } catch (error) {
    appendLog(`ERROR: ${error}`);
    showNotice(String(error), "error");
  } finally {
    state.zipBusy = false;
    render();
  }
}

bootstrap().catch((error) => {
  appRoot.innerHTML = `<pre class="fatal-error">${escapeHtml(String(error))}</pre>`;
});
