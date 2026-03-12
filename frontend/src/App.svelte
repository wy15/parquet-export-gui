<script lang="ts">
  import { afterUpdate, onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime";
  import type { appcore } from "../wailsjs/go/models";
  import {
    CheckExportOutput,
    CreateZipArchive,
    GetConfig,
    GetGeneratedFiles,
    StartTask,
  } from "../wailsjs/go/main/App";

  type ExportRequest = appcore.ExportRequest;
  type GeneratedFile = appcore.GeneratedFile;
  type Option = appcore.Option;

  type NoticeTone = "success" | "error";

  type NoticeState = {
    message: string;
    tone: NoticeTone;
  };

  type VisibilityState = {
    password: boolean;
    maxcomputeAccessKey: boolean;
    zipPassword: boolean;
  };

  type ExportConflictState = {
    request: ExportRequest;
    resolvedPath: string;
    suggestedPath: string;
  };

  type TaskEvent = {
    kind: string;
    type: string;
    message?: string;
    error?: string;
    outputPath?: string;
    rowsWritten?: number;
  };

  let backends: Option[] = [];
  let compressions: string[] = [];
  let defaultPorts: Record<string, number> = {};
  let running = false;
  let zipBusy = false;
  let rowsWritten = 0;
  let status = "待命";
  let logs: string[] = [];
  let notice: NoticeState | null = null;
  let exportConflict: ExportConflictState | null = null;
  let generatedFiles: GeneratedFile[] = [];
  let selectedFiles: string[] = [];
  let selectionInitialized = false;
  let logsExpanded = false;
  let zipEnabled = false;
  let zipPassword = "";
  let visibility: VisibilityState = {
    password: false,
    maxcomputeAccessKey: false,
    zipPassword: false,
  };
  let form: ExportRequest = {
    backend: "oracle",
    outputPath: "",
    conflictPolicy: "",
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
  };
  let entering = true;
  let fatalError = "";
  let logView: HTMLTextAreaElement | null = null;
  let noticeTimer: number | null = null;
  let lastRenderedLogCount = 0;
  let stickLogToBottom = true;

  $: isMaxCompute = form.backend === "maxcompute";
  $: allSelected = generatedFiles.length > 0 && selectedFiles.length === generatedFiles.length;
  $: selectedCount = selectedFiles.length;

  afterUpdate(() => {
    if (!logView) {
      lastRenderedLogCount = logs.length;
      return;
    }

    if (logs.length > lastRenderedLogCount || stickLogToBottom) {
      logView.scrollTop = logView.scrollHeight;
    }

    lastRenderedLogCount = logs.length;
  });

  onMount(() => {
    let active = true;
    const enterTimer = window.setTimeout(() => {
      entering = false;
    }, 500);
    let unsubscribe: (() => void) | undefined;

    (async () => {
      try {
        await waitForWailsBridge();
        const config = await GetConfig();
        if (!active) {
          return;
        }

        backends = config.backends ?? [];
        compressions = config.compressions ?? [];
        defaultPorts = config.defaultPorts ?? {};
        form = {
          ...form,
          ...config.defaultState,
        };
        await refreshGeneratedFiles();
        unsubscribe = EventsOn("task:event", (event: TaskEvent) => {
          handleTaskEvent(event);
        });
      } catch (error) {
        fatalError = toErrorMessage(error);
      }
    })();

    return () => {
      active = false;
      window.clearTimeout(enterTimer);
      if (noticeTimer) {
        window.clearTimeout(noticeTimer);
      }
      unsubscribe?.();
    };
  });

  function handleTaskEvent(event: TaskEvent) {
    switch (event.type) {
      case "running":
        running = event.message === "true";
        if (running) {
          status = event.kind === "export" ? "导出中" : "测试中";
          if (event.kind === "export") {
            rowsWritten = 0;
          }
          clearNotice();
        } else if (status === "导出中" || status === "测试中") {
          status = "待命";
        }
        break;
      case "log":
        appendLog(event.message ?? "");
        break;
      case "progress":
        if (event.kind === "export") {
          rowsWritten = event.rowsWritten ?? 0;
        }
        break;
      case "success":
        if (event.kind === "export") {
          status = "导出完成";
          rowsWritten = event.rowsWritten ?? 0;
          appendLog(`导出成功: ${event.outputPath ?? ""}`);
          showNotice(`导出成功: ${event.outputPath ?? ""}`, "success");
          void refreshGeneratedFiles();
        } else {
          status = "连接成功";
          appendLog("连接成功");
          showNotice("连接成功", "success");
        }
        break;
      case "error":
        status = "执行失败";
        appendLog(`ERROR: ${event.error ?? "未知错误"}`);
        showNotice(event.error ?? "未知错误", "error");
        break;
      default:
        appendLog(`WARN: 未知事件 ${event.type}`);
        break;
    }
  }

  function updateForm(patch: Partial<ExportRequest>) {
    form = {
      ...form,
      ...patch,
    };
  }

  function handleBackendChange(event: Event) {
    const backend = (event.currentTarget as HTMLSelectElement).value;
    updateForm({
      backend,
      port: defaultPorts[backend] ?? 0,
    });
  }

  function handleTableInput(event: Event) {
    const table = (event.currentTarget as HTMLInputElement).value;
    updateForm({
      table,
      outputPath: replaceOutputFilename(form.outputPath, table),
    });
  }

  function handlePortInput(event: Event) {
    const value = Number((event.currentTarget as HTMLInputElement).value);
    updateForm({ port: Number.isFinite(value) ? value : 0 });
  }

  function handleBatchSizeInput(event: Event) {
    const value = Number((event.currentTarget as HTMLInputElement).value);
    updateForm({ batchSize: Number.isFinite(value) ? value : 0 });
  }

  function toggleVisibility(key: keyof VisibilityState) {
    visibility = {
      ...visibility,
      [key]: !visibility[key],
    };
  }

  function fieldInputType(key: keyof VisibilityState) {
    return visibility[key] ? "text" : "password";
  }

  async function startTask(kind: string) {
    try {
      await StartTask(kind, sanitizeRequest());
    } catch (error) {
      const message = toErrorMessage(error);
      appendLog(`ERROR: ${message}`);
      status = "执行失败";
      showNotice(message, "error");
    }
  }

  async function startExportTask() {
    const request = sanitizeRequest();

    try {
      const check = await CheckExportOutput(request.outputPath);
      if (check.exists) {
        exportConflict = {
          request,
          resolvedPath: check.resolvedPath,
          suggestedPath: check.suggestedPath,
        };
        return;
      }

      updateForm({ outputPath: check.resolvedPath });
      await StartTask("export", {
        ...request,
        outputPath: check.resolvedPath,
        conflictPolicy: "overwrite",
      });
    } catch (error) {
      const message = toErrorMessage(error);
      appendLog(`ERROR: ${message}`);
      status = "执行失败";
      showNotice(message, "error");
    }
  }

  async function confirmExportConflict(action: "overwrite" | "rename") {
    if (!exportConflict) {
      return;
    }

    const { request, resolvedPath, suggestedPath } = exportConflict;
    const overwrite = action === "overwrite";
    const nextPath = overwrite ? resolvedPath : suggestedPath;

    exportConflict = null;
    updateForm({ outputPath: nextPath });

    try {
      await StartTask("export", {
        ...request,
        outputPath: nextPath,
        conflictPolicy: overwrite ? "overwrite" : "rename",
      });
    } catch (error) {
      const message = toErrorMessage(error);
      appendLog(`ERROR: ${message}`);
      status = "执行失败";
      showNotice(message, "error");
    }
  }

  function sanitizeRequest(): ExportRequest {
    return {
      ...form,
      batchSize: Number(form.batchSize) || 1,
      port: Number(form.port) || 0,
    };
  }

  async function refreshGeneratedFiles() {
    const files = await GetGeneratedFiles();
    const previousSelection = new Set(selectedFiles);
    const previousPaths = new Set(generatedFiles.map((file) => file.path));

    generatedFiles = files;
    selectedFiles = files
      .filter(
        (file) =>
          !selectionInitialized ||
          previousSelection.has(file.path) ||
          !previousPaths.has(file.path),
      )
      .map((file) => file.path);
    selectionInitialized = true;
  }

  function handleSelectAllChange(event: Event) {
    const checked = (event.currentTarget as HTMLInputElement).checked;
    selectedFiles = checked ? generatedFiles.map((file) => file.path) : [];
  }

  function handleFileSelection(path: string, event: Event) {
    const checked = (event.currentTarget as HTMLInputElement).checked;
    if (checked) {
      selectedFiles = selectedFiles.includes(path) ? selectedFiles : [...selectedFiles, path];
      return;
    }
    selectedFiles = selectedFiles.filter((item) => item !== path);
  }

  function handleZipEnabledChange(event: Event) {
    zipEnabled = (event.currentTarget as HTMLInputElement).checked;
    if (!zipEnabled) {
      zipPassword = "";
      visibility = {
        ...visibility,
        zipPassword: false,
      };
    }
  }

  async function createZipArchiveFromSelection() {
    if (!zipEnabled || selectedFiles.length === 0 || zipBusy) {
      return;
    }

    zipBusy = true;
    try {
      const result = await CreateZipArchive({
        files: selectedFiles,
        password: zipPassword,
      });
      appendLog(`ZIP 创建成功: ${result.outputPath}`);
      showNotice(`ZIP 创建成功: ${result.outputPath}`, "success");
    } catch (error) {
      const message = toErrorMessage(error);
      appendLog(`ERROR: ${message}`);
      showNotice(message, "error");
    } finally {
      zipBusy = false;
    }
  }

  function handleLogScroll() {
    if (!logView) {
      return;
    }
    const distanceToBottom = logView.scrollHeight - logView.scrollTop - logView.clientHeight;
    stickLogToBottom = distanceToBottom < 12;
  }

  function appendLog(message: string) {
    const timestamp = new Date().toLocaleTimeString("zh-CN", { hour12: false });
    logs = [...logs, `[${timestamp}] ${message}`].slice(-300);
  }

  function showNotice(message: string, tone: NoticeTone = "error") {
    notice = {
      message: String(message || ""),
      tone,
    };

    if (noticeTimer) {
      window.clearTimeout(noticeTimer);
    }

    noticeTimer = window.setTimeout(() => {
      notice = null;
      noticeTimer = null;
    }, 4800);
  }

  function clearNotice() {
    notice = null;
    if (noticeTimer) {
      window.clearTimeout(noticeTimer);
      noticeTimer = null;
    }
  }

  function onConflictBackdropClick(event: MouseEvent) {
    if (event.target === event.currentTarget) {
      exportConflict = null;
    }
  }

  function onConflictBackdropKeydown(event: KeyboardEvent) {
    if (event.key === "Enter" || event.key === " " || event.key === "Escape") {
      event.preventDefault();
      exportConflict = null;
    }
  }

  function replaceOutputFilename(currentPath: string, tableName: string) {
    const trimmedTable = String(tableName || "").trim();
    if (!trimmedTable) {
      return currentPath;
    }

    const normalizedPath = String(currentPath || "").trim();
    const separatorIndex = Math.max(
      normalizedPath.lastIndexOf("/"),
      normalizedPath.lastIndexOf("\\"),
    );
    const directory = separatorIndex >= 0 ? normalizedPath.slice(0, separatorIndex + 1) : "";
    const currentFile =
      separatorIndex >= 0 ? normalizedPath.slice(separatorIndex + 1) : normalizedPath;
    const extensionIndex = currentFile.lastIndexOf(".");
    const extension = extensionIndex > 0 ? currentFile.slice(extensionIndex) : ".parquet";

    return `${directory}${trimmedTable}${extension}`;
  }

  function numberWithCommas(value: number) {
    return new Intl.NumberFormat("zh-CN").format(value || 0);
  }

  function formatFileSize(value: number) {
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

  async function waitForWailsBridge(timeoutMs = 4000) {
    const startedAt = Date.now();
    while (Date.now() - startedAt < timeoutMs) {
      const bridgeWindow = window as Window & {
        runtime?: unknown;
        go?: { main?: { App?: unknown } };
      };
      if (bridgeWindow.runtime && bridgeWindow.go?.main?.App) {
        return;
      }
      await new Promise((resolve) => window.setTimeout(resolve, 16));
    }
    throw new Error("Wails runtime bridge did not initialise in time");
  }

  function toErrorMessage(error: unknown) {
    if (error instanceof Error) {
      return error.message;
    }
    return String(error);
  }
</script>

{#if fatalError}
  <pre class="fatal-error">{fatalError}</pre>
{:else}
  <div class:page-shell={true} class:is-entering={entering}>
    <section class="hero-card">
      <div class="hero-bar">
        <div>
          <h1>Parquet Export Studio</h1>
          <p class="hero-copy">
            从 Oracle / MySQL / PostgreSQL / MaxCompute 导出单表到本地 Parquet
          </p>
        </div>
        <div class="hero-metrics">
          <div class="hero-status">
            <span class:hero-status-dot={true} class:is-running={running}></span>
            {status}
          </div>
          <div class="hero-rows">已写入 {numberWithCommas(rowsWritten)} 行</div>
          {#if running}
            <div class="hero-progress">
              <div class="hero-progress-bar"></div>
            </div>
          {/if}
        </div>
      </div>

      <div class="panel-grid">
        <section class="panel panel-left">
          <h2 class="section-title">
            <span class="section-title-mark" aria-hidden="true"></span>
            <span>连接配置</span>
          </h2>

          <label class="field field-select">
            <span class="field-label">数据源</span>
            <span class="field-frame">
              <select value={form.backend} on:change={handleBackendChange}>
                {#each backends as option (option.value)}
                  <option value={option.value}>{option.label}</option>
                {/each}
              </select>
            </span>
          </label>

          {#if isMaxCompute}
            <label class="field">
              <span class="field-label">Endpoint</span>
              <span class="field-frame">
                <input
                  bind:value={form.maxcomputeEndpoint}
                  placeholder="https://service.cn-hangzhou.maxcompute.aliyun.com/api"
                />
              </span>
            </label>

            <label class="field">
              <span class="field-label">Project</span>
              <span class="field-frame">
                <input bind:value={form.maxcomputeProject} />
              </span>
            </label>

            <div class="field-row">
              <label class="field grow">
                <span class="field-label">Access ID</span>
                <span class="field-frame">
                  <input bind:value={form.maxcomputeAccessId} />
                </span>
              </label>

              <label class="field grow field-password">
                <span class="field-label">Access Key</span>
                <span class="field-frame">
                  <input
                    bind:value={form.maxcomputeAccessKey}
                    type={fieldInputType("maxcomputeAccessKey")}
                  />
                  <button
                    type="button"
                    class="field-visibility-toggle"
                    aria-label={visibility.maxcomputeAccessKey ? "隐藏" : "显示"}
                    aria-pressed={visibility.maxcomputeAccessKey}
                    title={visibility.maxcomputeAccessKey ? "隐藏" : "显示"}
                    on:click={() => toggleVisibility("maxcomputeAccessKey")}
                  >
                    <svg viewBox="0 0 20 20" aria-hidden="true">
                      <path
                        d="M2.4 10C3.86 6.95 6.66 5 10 5C13.34 5 16.14 6.95 17.6 10C16.14 13.05 13.34 15 10 15C6.66 15 3.86 13.05 2.4 10Z"
                      />
                      <circle cx="10" cy="10" r="2.4" />
                      {#if !visibility.maxcomputeAccessKey}
                        <path d="M4 4L16 16" />
                      {/if}
                    </svg>
                  </button>
                </span>
              </label>
            </div>

            <div class="field-row">
              <label class="field grow">
                <span class="field-label">Schema (optional)</span>
                <span class="field-frame">
                  <input bind:value={form.schema} />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Table</span>
                <span class="field-frame">
                  <input value={form.table} on:input={handleTableInput} />
                </span>
              </label>
            </div>

            <label class="field">
              <span class="field-label">Partition Spec (optional)</span>
              <span class="field-frame">
                <input bind:value={form.partitionSpec} placeholder="ds='2026-03-07', region='cn'" />
              </span>
            </label>
          {:else}
            <label class="field">
              <span class="field-label">Host</span>
              <span class="field-frame">
                <input bind:value={form.host} />
              </span>
            </label>

            <label class="field">
              <span class="field-label">Port</span>
              <span class="field-frame">
                <input type="number" value={form.port} on:input={handlePortInput} />
              </span>
            </label>

            <div class="field-row">
              <label class="field grow">
                <span class="field-label">Username</span>
                <span class="field-frame">
                  <input bind:value={form.username} />
                </span>
              </label>

              <label class="field grow field-password">
                <span class="field-label">Password</span>
                <span class="field-frame">
                  <input bind:value={form.password} type={fieldInputType("password")} />
                  <button
                    type="button"
                    class="field-visibility-toggle"
                    aria-label={visibility.password ? "隐藏" : "显示"}
                    aria-pressed={visibility.password}
                    title={visibility.password ? "隐藏" : "显示"}
                    on:click={() => toggleVisibility("password")}
                  >
                    <svg viewBox="0 0 20 20" aria-hidden="true">
                      <path
                        d="M2.4 10C3.86 6.95 6.66 5 10 5C13.34 5 16.14 6.95 17.6 10C16.14 13.05 13.34 15 10 15C6.66 15 3.86 13.05 2.4 10Z"
                      />
                      <circle cx="10" cy="10" r="2.4" />
                      {#if !visibility.password}
                        <path d="M4 4L16 16" />
                      {/if}
                    </svg>
                  </button>
                </span>
              </label>
            </div>

            <label class="field">
              <span class="field-label">Database / Service Name</span>
              <span class="field-frame">
                <input bind:value={form.database} />
              </span>
            </label>

            <div class="field-row">
              <label class="field grow">
                <span class="field-label">Schema (optional)</span>
                <span class="field-frame">
                  <input bind:value={form.schema} />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Table</span>
                <span class="field-frame">
                  <input value={form.table} on:input={handleTableInput} />
                </span>
              </label>
            </div>
          {/if}
        </section>

        <div class="panel-stack">
          <section class="panel">
            <h2 class="section-title">
              <span class="section-title-mark" aria-hidden="true"></span>
              <span>导出参数</span>
            </h2>

            <label class="field">
              <span class="field-label">输出 Parquet 路径</span>
              <span class="field-frame">
                <input bind:value={form.outputPath} />
              </span>
            </label>

            <div class="field-row">
              <label class="field grow">
                <span class="field-label">批次大小</span>
                <span class="field-frame">
                  <input
                    min="1"
                    step="1000"
                    type="number"
                    value={form.batchSize}
                    on:input={handleBatchSizeInput}
                  />
                </span>
              </label>

              <label class="field grow field-select">
                <span class="field-label">压缩</span>
                <span class="field-frame">
                  <select bind:value={form.compression}>
                    {#each compressions as option (option)}
                      <option value={option}>{option}</option>
                    {/each}
                  </select>
                </span>
              </label>
            </div>

            <p class="panel-note">单次写入的行数。调大更快，调小更省内存。</p>

            <div class="button-row">
              <button
                class="button button-secondary"
                disabled={running}
                on:click={() => startTask("test")}
              >
                测试连接
              </button>
              <button class="button button-primary" disabled={running} on:click={startExportTask}>
                <span class="button-icon">▶</span>开始导出
              </button>
            </div>
          </section>

          <section class="panel panel-log">
            <button
              type="button"
              class="panel-toggle"
              aria-expanded={logsExpanded}
              on:click={() => (logsExpanded = !logsExpanded)}
            >
              <span class="panel-toggle-copy">
                <span class="panel-toggle-eyebrow">调试输出</span>
                <span class="panel-toggle-title">运行日志</span>
              </span>
              <span class="panel-toggle-meta">
                <span class="log-badge">{logs.length}</span>
                <span class="panel-toggle-state">{logsExpanded ? "收起" : "展开"}</span>
                <svg
                  class="panel-chevron"
                  class:is-open={logsExpanded}
                  width="14"
                  height="14"
                  viewBox="0 0 14 14"
                  fill="none"
                  aria-hidden="true"
                >
                  <path
                    d="M3.5 5.25L7 8.75L10.5 5.25"
                    stroke="currentColor"
                    stroke-width="1.6"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                </svg>
              </span>
            </button>

            <div class="log-collapse" class:is-open={logsExpanded}>
              <textarea
                bind:this={logView}
                readonly
                placeholder="日志会显示在这里"
                aria-label="运行日志"
                on:scroll={handleLogScroll}
                value={logs.join("\n")}
              ></textarea>
            </div>
          </section>

          <section class="panel">
            <h2 class="section-title">
              <span class="section-title-mark" aria-hidden="true"></span>
              <span>本次生成的 Parquet</span>
            </h2>

            <div class="generated-toolbar">
              <label class="list-checkbox master-checkbox">
                <input
                  type="checkbox"
                  checked={allSelected}
                  disabled={generatedFiles.length === 0}
                  aria-label="全选文件"
                  on:change={handleSelectAllChange}
                />
                <span>全选文件</span>
              </label>
              <span class="generated-meta">已选 {selectedCount} / {generatedFiles.length}</span>
            </div>

            <div class="generated-list" class:is-empty={generatedFiles.length === 0}>
              {#if generatedFiles.length === 0}
                <p class="empty-copy">当前会话还没有生成 parquet 文件。</p>
              {:else}
                {#each generatedFiles as file (file.path)}
                  <label
                    class="generated-item"
                    class:is-selected={selectedFiles.includes(file.path)}
                  >
                    <span class="list-checkbox">
                      <input
                        class="generated-file-checkbox"
                        type="checkbox"
                        checked={selectedFiles.includes(file.path)}
                        on:change={(event) => handleFileSelection(file.path, event)}
                      />
                      <span class="generated-file-copy">
                        <strong>{file.name}</strong>
                      </span>
                    </span>
                    <span class="generated-file-meta">{formatFileSize(file.size)}</span>
                  </label>
                {/each}
              {/if}
            </div>

            <div class="zip-controls">
              <div class="zip-surface">
                <label class="list-checkbox">
                  <input
                    type="checkbox"
                    checked={zipEnabled}
                    disabled={generatedFiles.length === 0}
                    on:change={handleZipEnabledChange}
                  />
                  <span>ZIP 压缩选中文件</span>
                </label>

                {#if zipEnabled}
                  <label class="field field-password compact-field">
                    <span class="field-label">加密密码（可选）</span>
                    <span class="field-frame">
                      <input
                        bind:value={zipPassword}
                        type={fieldInputType("zipPassword")}
                        placeholder="留空则生成不加密 ZIP"
                      />
                      <button
                        type="button"
                        class="field-visibility-toggle"
                        aria-label={visibility.zipPassword ? "隐藏" : "显示"}
                        aria-pressed={visibility.zipPassword}
                        title={visibility.zipPassword ? "隐藏" : "显示"}
                        on:click={() => toggleVisibility("zipPassword")}
                      >
                        <svg viewBox="0 0 20 20" aria-hidden="true">
                          <path
                            d="M2.4 10C3.86 6.95 6.66 5 10 5C13.34 5 16.14 6.95 17.6 10C16.14 13.05 13.34 15 10 15C6.66 15 3.86 13.05 2.4 10Z"
                          />
                          <circle cx="10" cy="10" r="2.4" />
                          {#if !visibility.zipPassword}
                            <path d="M4 4L16 16" />
                          {/if}
                        </svg>
                      </button>
                    </span>
                  </label>
                {/if}

                <div class="zip-actions">
                  <button
                    class="button button-secondary"
                    disabled={!zipEnabled || selectedCount === 0 || zipBusy}
                    on:click={createZipArchiveFromSelection}
                  >
                    {zipBusy ? "正在打包…" : "创建 ZIP"}
                  </button>
                  <p class="zip-note">ZIP 将保存到首个选中文件所在目录。</p>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </section>

    {#if notice}
      <div class={`notice-banner notice-${notice.tone}`} role="status" aria-live="polite">
        <span class="notice-icon" aria-hidden="true">{notice.tone === "success" ? "✓" : "⚠"}</span>
        <span class="notice-message">{notice.message}</span>
      </div>
    {/if}

    {#if exportConflict}
      <div
        class="dialog-backdrop"
        role="button"
        tabindex="0"
        aria-label="关闭导出冲突对话框"
        on:click={onConflictBackdropClick}
        on:keydown={onConflictBackdropKeydown}
      >
        <div
          class="dialog-card"
          role="dialog"
          aria-modal="true"
          aria-labelledby="exportConflictTitle"
        >
          <h3 id="exportConflictTitle">输出文件已存在</h3>
          <p class="dialog-copy">目标路径已经存在同名文件。请选择直接覆盖，或自动改名后再导出。</p>
          <div class="dialog-paths">
            <div class="dialog-path-row">
              <span>当前目标</span>
              <code>{exportConflict.resolvedPath}</code>
            </div>
            <div class="dialog-path-row">
              <span>自动改名</span>
              <code>{exportConflict.suggestedPath}</code>
            </div>
          </div>
          <div class="dialog-actions">
            <button class="button button-ghost" on:click={() => (exportConflict = null)}
              >取消</button
            >
            <button
              class="button button-secondary"
              on:click={() => confirmExportConflict("rename")}
            >
              自动改名
            </button>
            <button
              class="button button-primary"
              on:click={() => confirmExportConflict("overwrite")}
            >
              覆盖原文件
            </button>
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}

<style lang="less">
  .page-shell {
    padding: 44px 32px 56px;
  }

  .hero-card,
  .panel {
    background: var(--panel-bg);
    backdrop-filter: blur(14px);
    border: 1px solid var(--panel-border);
    border-radius: 28px;
    box-shadow: var(--shadow);
  }

  .hero-card {
    max-width: 1220px;
    margin: 0 auto;
    padding: 36px 34px 40px;
    position: relative;
    overflow: hidden;
  }

  .hero-card::before {
    content: "";
    position: absolute;
    inset: 0;
    background:
      radial-gradient(circle at top right, rgba(47, 122, 118, 0.08), transparent 26%),
      linear-gradient(180deg, rgba(255, 255, 255, 0.32), transparent 22%);
    pointer-events: none;
  }

  .hero-bar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 24px;
  }

  .hero-bar h1 {
    margin: 0;
    font-size: 34px;
    line-height: 1.05;
    letter-spacing: -0.04em;
    font-weight: 800;
  }

  .hero-copy {
    margin: 10px 0 0;
    font-size: 15px;
  }

  .panel-note {
    margin: 6px 0 0;
    max-width: 720px;
    color: #6f7b8d;
    font-size: 12.5px;
    line-height: 1.55;
  }

  .hero-metrics {
    display: grid;
    gap: 10px;
    min-width: 180px;
    text-align: right;
  }

  .hero-status {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    color: var(--primary);
    font-weight: 700;
    font-size: 15px;
  }

  .hero-status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--primary);
    flex: 0 0 auto;
  }

  .hero-status-dot.is-running {
    animation: pulse-dot 1.4s ease-in-out infinite;
  }

  @keyframes pulse-dot {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }

    50% {
      opacity: 0.4;
      transform: scale(0.75);
    }
  }

  .hero-rows {
    color: var(--muted);
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    font-size: 15px;
  }

  .hero-progress {
    height: 3px;
    border-radius: 2px;
    background: rgba(47, 122, 118, 0.12);
    overflow: hidden;
    margin-top: 4px;
  }

  .hero-progress-bar {
    width: 30%;
    height: 100%;
    border-radius: 2px;
    background: linear-gradient(90deg, var(--primary), #4ab0a8);
    animation: indeterminate 1.6s ease-in-out infinite;
  }

  @keyframes indeterminate {
    0% {
      transform: translateX(-100%);
    }

    100% {
      transform: translateX(430%);
    }
  }

  .panel-grid {
    display: grid;
    grid-template-columns: minmax(340px, 400px) minmax(0, 1fr);
    gap: 22px;
    align-items: start;
  }

  .panel-stack {
    display: grid;
    gap: 8px;
  }

  .panel {
    padding: 22px 20px 20px;
    position: relative;
    overflow: hidden;
    background: linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.96) 0%,
      rgba(249, 251, 252, 0.92) 100%
    );
    border-color: rgba(148, 163, 184, 0.24);
    box-shadow:
      0 18px 38px rgba(148, 163, 184, 0.11),
      inset 0 1px 0 rgba(255, 255, 255, 0.8);
  }

  .panel::before {
    content: "";
    position: absolute;
    inset: 0 auto auto 0;
    width: 100%;
    height: 1px;
    background: linear-gradient(90deg, rgba(47, 122, 118, 0.18), rgba(47, 122, 118, 0));
    pointer-events: none;
  }

  .section-title {
    margin: 0 0 18px;
    display: flex;
    align-items: center;
    gap: 9px;
    font-size: 18px;
    font-weight: 800;
  }

  .section-title-mark {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: linear-gradient(180deg, rgba(47, 122, 118, 0.95) 0%, rgba(45, 94, 117, 0.82) 100%);
    box-shadow:
      0 0 0 4px rgba(47, 122, 118, 0.07),
      0 4px 10px rgba(47, 122, 118, 0.14);
    flex: 0 0 auto;
  }

  .field,
  .field-row {
    display: flex;
  }

  .field {
    flex-direction: column;
    position: relative;
    gap: 4px;
    margin-bottom: 12px;
  }

  .field-row {
    gap: 10px;
  }

  .grow {
    flex: 1;
  }

  .field-label {
    color: #5a6478;
    font-size: 12px;
    font-weight: 700;
    line-height: 1.1;
    pointer-events: none;
    padding-left: 3px;
  }

  .field-frame {
    position: relative;
    display: block;
    padding: 8px 12px 6px;
    border: 1px solid rgba(148, 163, 184, 0.28);
    border-radius: 14px;
    background: linear-gradient(
      180deg,
      rgba(247, 250, 252, 0.98) 0%,
      rgba(255, 255, 255, 0.96) 100%
    );
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.86),
      0 6px 14px rgba(148, 163, 184, 0.07);
    transition:
      border-color 0.15s ease,
      box-shadow 0.15s ease,
      background-color 0.15s ease;
  }

  .field:focus-within .field-frame {
    border-color: rgba(47, 122, 118, 0.34);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.92),
      0 0 0 3px rgba(47, 122, 118, 0.07),
      0 8px 18px rgba(47, 122, 118, 0.06);
    background: linear-gradient(180deg, rgba(248, 252, 251, 1) 0%, rgba(255, 255, 255, 1) 100%);
  }

  input,
  select,
  textarea,
  button {
    font: inherit;
  }

  input,
  select,
  textarea {
    width: 100%;
    padding: 1px 2px 2px;
    border-radius: 10px;
    border: none;
    background: transparent;
    color: var(--ink);
    outline: none;
    box-shadow: none;
    transition:
      color 0.15s ease,
      background-color 0.15s ease;
    appearance: none;
  }

  input:focus,
  select:focus,
  textarea:focus {
    background: transparent;
  }

  input::placeholder,
  textarea::placeholder {
    color: #b0b7c4;
  }

  input[type="number"] {
    padding-right: 2px;
  }

  select {
    padding-right: 30px;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8' fill='none'%3E%3Cpath d='M1 1.5L6 6.5L11 1.5' stroke='%23778292' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 1px center;
    background-size: 12px 8px;
  }

  .field-password input {
    padding-right: 28px;
  }

  .compact-field {
    margin-top: 12px;
  }

  .field-visibility-toggle {
    position: absolute;
    right: 1px;
    top: 50%;
    transform: translateY(calc(-50% - 1px));
    width: 28px;
    height: 28px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: #778292;
    cursor: pointer;
    opacity: 0.7;
  }

  .field-visibility-toggle:hover {
    opacity: 1;
    background: rgba(148, 163, 184, 0.12);
  }

  .field-visibility-toggle:focus-visible {
    opacity: 1;
    outline: 2px solid rgba(47, 122, 118, 0.28);
    outline-offset: 1px;
  }

  :global(.field-visibility-toggle svg) {
    width: 18px;
    height: 18px;
  }

  :global(.field-visibility-toggle path),
  :global(.field-visibility-toggle circle) {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.4;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  textarea {
    min-height: 222px;
    resize: vertical;
    padding: 12px 14px;
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    font-size: 13px;
    line-height: 1.5;
    border: 1px solid rgba(148, 163, 184, 0.22);
    border-color: rgba(148, 163, 184, 0.22);
    background: rgba(248, 250, 251, 0.95);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
  }

  textarea:focus {
    border-color: rgba(47, 122, 118, 0.52);
    box-shadow: 0 0 0 4px rgba(47, 122, 118, 0.08);
    background: #fff;
  }

  .panel-log {
    padding: 16px;
    border-color: rgba(47, 122, 118, 0.18);
  }

  .panel-toggle {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    gap: 12px;
    padding: 12px 14px;
    border: 1px solid rgba(148, 163, 184, 0.18);
    border-radius: 16px;
    background: linear-gradient(
      180deg,
      rgba(248, 250, 250, 0.96) 0%,
      rgba(252, 253, 253, 0.98) 100%
    );
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.84),
      0 8px 18px rgba(148, 163, 184, 0.08);
    color: inherit;
    text-align: left;
    cursor: pointer;
    user-select: none;
    margin: 0;
    transition:
      color 0.15s ease,
      border-color 0.15s ease,
      box-shadow 0.15s ease,
      transform 0.15s ease;
  }

  .panel-toggle:hover {
    color: var(--primary);
    border-color: rgba(47, 122, 118, 0.24);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.88),
      0 10px 20px rgba(47, 122, 118, 0.08);
  }

  .panel-toggle:focus-visible {
    outline: none;
    border-color: rgba(47, 122, 118, 0.4);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.88),
      0 0 0 4px rgba(47, 122, 118, 0.08);
  }

  .panel-toggle-copy {
    display: grid;
    gap: 2px;
  }

  .panel-toggle-eyebrow {
    display: none;
  }

  .panel-toggle-title {
    font-size: 17px;
    font-weight: 800;
    line-height: 1;
  }

  .panel-toggle-meta {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--muted);
    flex: 0 0 auto;
    justify-content: flex-end;
  }

  .panel-toggle-state {
    font-size: 11.5px;
    font-weight: 700;
    min-width: 24px;
    text-align: right;
  }

  .panel-chevron {
    color: var(--muted);
    transition: transform 0.25s cubic-bezier(0.22, 1, 0.36, 1);
    flex: 0 0 auto;
    margin-left: 2px;
  }

  .panel-chevron.is-open {
    transform: rotate(180deg);
  }

  .log-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 22px;
    height: 22px;
    padding: 0 7px;
    border-radius: 999px;
    background: rgba(47, 122, 118, 0.1);
    color: var(--primary);
    font-size: 11px;
    font-weight: 800;
    line-height: 1;
  }

  .log-collapse {
    max-height: 0;
    overflow: hidden;
    transition: max-height 0.3s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .log-collapse.is-open {
    max-height: 400px;
    margin-top: 12px;
  }

  .button-row {
    display: flex;
    gap: 10px;
    margin-top: 14px;
  }

  .button {
    border: none;
    border-radius: 10px;
    padding: 10px 17px;
    cursor: pointer;
    font-weight: 700;
    font-size: 14px;
    transition:
      transform 0.15s ease,
      opacity 0.15s ease,
      filter 0.15s ease,
      box-shadow 0.15s ease,
      background-color 0.15s ease;
  }

  .button-icon {
    margin-right: 5px;
  }

  .button:hover:not(:disabled) {
    transform: translateY(-1px);
    filter: brightness(1.02);
  }

  .button:active:not(:disabled) {
    transform: translateY(0);
  }

  .button:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .button-primary {
    background: linear-gradient(180deg, #41958f 0%, #2f7a76 100%);
    color: white;
    box-shadow:
      0 5px 14px rgba(47, 122, 118, 0.24),
      inset 0 1px 0 rgba(255, 255, 255, 0.14);
  }

  .button-primary:hover:not(:disabled) {
    box-shadow:
      0 7px 18px rgba(47, 122, 118, 0.28),
      inset 0 1px 0 rgba(255, 255, 255, 0.16);
  }

  .button-secondary {
    background: rgba(255, 255, 255, 0.88);
    color: var(--secondary);
    border: 1px solid rgba(45, 94, 117, 0.28);
  }

  .button-secondary:hover:not(:disabled) {
    background: rgba(45, 94, 117, 0.05);
    border-color: rgba(45, 94, 117, 0.38);
  }

  .button-ghost {
    background: rgba(148, 163, 184, 0.14);
    color: var(--ink);
  }

  .generated-toolbar,
  .generated-item,
  .zip-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .generated-toolbar {
    margin-bottom: 12px;
  }

  .master-checkbox span:last-child {
    font-weight: 700;
    color: var(--ink);
  }

  .generated-meta,
  .zip-note,
  .empty-copy,
  .generated-file-meta {
    color: var(--muted);
    font-size: 13px;
  }

  .generated-list {
    display: grid;
    gap: 10px;
    margin-bottom: 14px;
  }

  .generated-list.is-empty {
    padding: 12px 0 4px;
  }

  .generated-item {
    padding: 12px 14px;
    border: 1px solid rgba(148, 163, 184, 0.24);
    border-radius: 14px;
    background: linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.94) 0%,
      rgba(249, 251, 252, 0.92) 100%
    );
    transition:
      border-color 0.15s ease,
      box-shadow 0.15s ease,
      transform 0.15s ease;
  }

  .generated-item:hover {
    border-color: rgba(47, 122, 118, 0.24);
    box-shadow: 0 6px 18px rgba(148, 163, 184, 0.1);
  }

  .generated-item.is-selected {
    border-color: rgba(47, 122, 118, 0.34);
    background: linear-gradient(
      180deg,
      rgba(245, 251, 250, 0.98) 0%,
      rgba(251, 252, 253, 0.96) 100%
    );
  }

  .list-checkbox {
    display: inline-flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }

  .list-checkbox input[type="checkbox"] {
    width: 22px;
    height: 22px;
    min-width: 22px;
    padding: 0;
    margin: 0;
    flex: 0 0 auto;
    border: 1px solid rgba(113, 129, 151, 0.54);
    border-radius: 7px;
    background: linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.98) 0%,
      rgba(241, 245, 249, 0.96) 100%
    );
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.95),
      0 1px 2px rgba(15, 23, 42, 0.08);
    appearance: none;
    -webkit-appearance: none;
    cursor: pointer;
    position: relative;
    transition:
      border-color 0.15s ease,
      background-color 0.15s ease,
      box-shadow 0.15s ease,
      transform 0.12s ease;
  }

  .list-checkbox input[type="checkbox"]:hover:not(:disabled) {
    border-color: rgba(47, 122, 118, 0.52);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.96),
      0 0 0 4px rgba(47, 122, 118, 0.08);
  }

  .list-checkbox input[type="checkbox"]:focus-visible {
    outline: none;
    border-color: rgba(47, 122, 118, 0.72);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.96),
      0 0 0 4px rgba(47, 122, 118, 0.12);
  }

  .list-checkbox input[type="checkbox"]:checked {
    border-color: #2f7a76;
    background: linear-gradient(180deg, #3e8f89 0%, #2f7a76 100%);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.18),
      0 8px 14px rgba(47, 122, 118, 0.2);
  }

  .list-checkbox input[type="checkbox"]:checked::after {
    content: "";
    position: absolute;
    left: 7px;
    top: 3px;
    width: 5px;
    height: 10px;
    border: solid #fff;
    border-width: 0 2.5px 2.5px 0;
    transform: rotate(45deg);
  }

  .list-checkbox input[type="checkbox"]:active:not(:disabled) {
    transform: scale(0.96);
  }

  .list-checkbox input[type="checkbox"]:disabled {
    cursor: not-allowed;
    opacity: 0.52;
    box-shadow: none;
  }

  .list-checkbox span:last-child {
    line-height: 1.2;
  }

  .generated-file-copy {
    min-width: 0;
    display: flex;
    align-items: center;
  }

  .generated-file-copy strong {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 14px;
    font-weight: 800;
    color: var(--ink);
  }

  .generated-file-meta {
    white-space: nowrap;
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    font-size: 11px;
  }

  .zip-controls {
    padding-top: 6px;
  }

  .zip-surface {
    padding: 14px 16px 16px;
    border: none;
    border-left: 3px solid rgba(47, 122, 118, 0.22);
    border-radius: 0 12px 12px 0;
    background: rgba(247, 250, 250, 0.7);
  }

  .zip-actions {
    margin-top: 12px;
  }

  .zip-note,
  .empty-copy {
    margin: 0;
  }

  .zip-note {
    text-align: right;
  }

  .fatal-error {
    max-width: 960px;
    margin: 40px auto;
    white-space: pre-wrap;
  }

  .notice-banner {
    position: fixed;
    left: 50%;
    bottom: 28px;
    z-index: 1000;
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: min(460px, calc(100vw - 32px));
    max-width: calc(100vw - 32px);
    padding: 12px 16px;
    border-radius: 8px;
    box-shadow: 0 18px 40px rgba(15, 23, 42, 0.18);
    color: #fff;
    animation: notice-in 0.32s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
  }

  @keyframes notice-in {
    from {
      transform: translateX(-50%) translateY(24px);
      opacity: 0;
    }

    to {
      transform: translateX(-50%) translateY(0);
      opacity: 1;
    }
  }

  .notice-error {
    background: linear-gradient(135deg, #c94e3c 0%, #c24133 100%);
  }

  .notice-success {
    background: linear-gradient(135deg, #2f7a76 0%, #2a6764 100%);
  }

  .notice-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.18);
    font-size: 13px;
    font-weight: 800;
    flex: 0 0 auto;
  }

  .notice-message {
    line-height: 1.4;
    word-break: break-word;
  }

  .dialog-backdrop {
    position: fixed;
    inset: 0;
    z-index: 1100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    background: rgba(15, 23, 42, 0.18);
    backdrop-filter: blur(10px);
  }

  .dialog-card {
    width: min(560px, 100%);
    padding: 24px;
    border: 1px solid rgba(148, 163, 184, 0.18);
    border-radius: 22px;
    background: linear-gradient(
      180deg,
      rgba(253, 254, 254, 0.98) 0%,
      rgba(246, 249, 249, 0.96) 100%
    );
    box-shadow: 0 28px 60px rgba(15, 23, 42, 0.18);
  }

  .dialog-card h3 {
    margin: 0 0 10px;
    font-size: 20px;
    font-weight: 800;
  }

  .dialog-copy {
    margin: 0;
    color: var(--muted);
    line-height: 1.55;
  }

  .dialog-paths {
    display: grid;
    gap: 10px;
    margin-top: 18px;
  }

  .dialog-path-row {
    display: grid;
    gap: 6px;
    padding: 12px 14px;
    border: 1px solid rgba(148, 163, 184, 0.18);
    border-radius: 14px;
    background: rgba(255, 255, 255, 0.72);
  }

  .dialog-path-row span {
    color: var(--muted);
    font-size: 12px;
    font-weight: 700;
  }

  .dialog-path-row code {
    overflow-wrap: anywhere;
    color: var(--ink);
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    font-size: 12px;
  }

  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 18px;
  }

  .is-entering .panel {
    animation: panel-in 0.38s cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  .is-entering .panel-stack .panel:nth-child(1) {
    animation-delay: 0.04s;
  }

  .is-entering .panel-stack .panel:nth-child(2) {
    animation-delay: 0.1s;
  }

  .is-entering .panel-stack .panel:nth-child(3) {
    animation-delay: 0.16s;
  }

  @keyframes panel-in {
    from {
      opacity: 0;
      transform: translateY(12px);
    }

    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .is-entering .generated-item {
    animation: item-in 0.28s ease both;
  }

  @keyframes item-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }

    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (max-width: 1120px) {
    .hero-bar,
    .panel-grid {
      display: grid;
      grid-template-columns: 1fr;
    }

    .hero-metrics {
      text-align: left;
    }

    .hero-status {
      justify-content: flex-start;
    }
  }

  @media (max-width: 720px) {
    .page-shell {
      padding: 16px;
    }

    .hero-card,
    .panel {
      padding: 20px;
      border-radius: 22px;
    }

    .hero-bar h1 {
      font-size: 30px;
    }

    .field-row,
    .button-row,
    .generated-item,
    .zip-actions {
      flex-direction: column;
    }

    .generated-item,
    .zip-actions {
      align-items: flex-start;
    }

    .notice-banner {
      left: 16px;
      right: 16px;
      bottom: 16px;
      transform: none;
      min-width: 0;
    }

    .dialog-actions {
      flex-direction: column-reverse;
      align-items: stretch;
    }
  }
</style>
