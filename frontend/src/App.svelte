<script lang="ts">
  import { afterUpdate, onMount } from "svelte";
  import { EventsOn } from "../wailsjs/runtime/runtime";
  import type { appcore } from "../wailsjs/go/models";
  import {
    CheckExportOutput,
    ClearGeneratedFiles,
    CreateZipArchive,
    GetConfig,
    GetGeneratedFiles,
    PreviewExport,
    StartTask,
  } from "../wailsjs/go/main/App";

  type ExportRequest = appcore.ExportRequest;
  type ExportPreview = appcore.ExportPreview;
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

  type SchemaExportConfirmationState = {
    request: ExportRequest;
    schema: string;
    tableCount: number;
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
  let schemaExportConfirmation: SchemaExportConfirmationState | null = null;
  let generatedFiles: GeneratedFile[] = [];
  let selectedFiles: string[] = [];
  let selectionInitialized = false;
  let logsExpanded = false;
  let zipEnabled = false;
  let zipPassword = "";
  let configDialogOpen = false;
  let configDraft: ExportRequest | null = null;
  let visibility: VisibilityState = {
    password: false,
    maxcomputeAccessKey: false,
    zipPassword: false,
  };
  let form: ExportRequest = {
    backend: "oracle",
    exportMode: "table",
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

  $: isOracle = form.backend === "oracle";
  $: isMaxCompute = form.backend === "maxcompute";
  $: isOracleSchemaMode = isOracle && form.exportMode === "schema";
  $: dialogForm = configDraft ?? form;
  $: dialogIsOracle = dialogForm.backend === "oracle";
  $: dialogIsMaxCompute = dialogForm.backend === "maxcompute";
  $: dialogIsOracleSchemaMode = dialogIsOracle && dialogForm.exportMode === "schema";
  $: allSelected = generatedFiles.length > 0 && selectedFiles.length === generatedFiles.length;
  $: selectedCount = selectedFiles.length;
  $: backendLabel = backends.find((option) => option.value === form.backend)?.label ?? form.backend;
  $: connectionSummary = isMaxCompute
    ? [form.maxcomputeProject, form.maxcomputeEndpoint].filter(Boolean).join(" · ")
    : [form.host, form.database].filter(Boolean).join(" · ");
  $: statusTone = running
    ? "running"
    : status.includes("失败")
      ? "error"
      : status === "导出完成" || status === "连接成功"
        ? "success"
        : "idle";

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

  function updateConfigDraft(patch: Partial<ExportRequest>) {
    if (!configDraft) {
      return;
    }

    configDraft = {
      ...configDraft,
      ...patch,
    };
  }

  function handleBackendChange(event: Event) {
    if (!configDraft) {
      return;
    }

    const backend = (event.currentTarget as HTMLSelectElement).value;
    const exportMode = backend === "oracle" ? configDraft.exportMode || "table" : "table";
    updateConfigDraft({
      backend,
      exportMode,
      port: defaultPorts[backend] ?? 0,
    });
  }

  function handleExportModeChange(event: Event) {
    if (!configDraft) {
      return;
    }

    const exportMode = (event.currentTarget as HTMLSelectElement).value;
    if (exportMode === "schema") {
      updateConfigDraft({
        exportMode,
        outputPath: replaceOutputDirectory(configDraft.outputPath, configDraft.schema),
      });
      return;
    }

    updateConfigDraft({
      exportMode,
      outputPath: replaceOutputFilename(configDraft.outputPath, configDraft.table),
    });
  }

  function handleTableInput(event: Event) {
    if (!configDraft) {
      return;
    }

    const table = (event.currentTarget as HTMLInputElement).value;
    updateConfigDraft({
      table,
      outputPath: dialogIsOracleSchemaMode
        ? configDraft.outputPath
        : replaceOutputFilename(configDraft.outputPath, table),
    });
  }

  function handleSchemaInput(event: Event) {
    if (!configDraft) {
      return;
    }

    const schema = (event.currentTarget as HTMLInputElement).value;
    updateConfigDraft({
      schema,
      outputPath: dialogIsOracleSchemaMode
        ? replaceOutputDirectory(configDraft.outputPath, schema)
        : configDraft.outputPath,
    });
  }

  function handlePortInput(event: Event) {
    const value = Number((event.currentTarget as HTMLInputElement).value);
    updateConfigDraft({ port: Number.isFinite(value) ? value : 0 });
  }

  function handleConfigTextInput(field: keyof ExportRequest, event: Event) {
    updateConfigDraft({
      [field]: (event.currentTarget as HTMLInputElement).value,
    } as Partial<ExportRequest>);
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
      const preview = await PreviewExport(request);
      if (preview.requiresConfirmation) {
        schemaExportConfirmation = {
          request,
          schema: preview.schema || request.schema,
          tableCount: preview.tableCount,
        };
        return;
      }

      await runExport(request, preview);
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

  async function confirmSchemaExport() {
    if (!schemaExportConfirmation) {
      return;
    }

    const { request } = schemaExportConfirmation;
    schemaExportConfirmation = null;

    try {
      await runExport(request);
    } catch (error) {
      const message = toErrorMessage(error);
      appendLog(`ERROR: ${message}`);
      status = "执行失败";
      showNotice(message, "error");
    }
  }

  function sanitizeRequest(request: ExportRequest = form): ExportRequest {
    const exportMode = request.backend === "oracle" ? request.exportMode || "table" : "table";
    return {
      ...request,
      exportMode,
      conflictPolicy: isOracleSchemaRequest(request)
        ? request.conflictPolicy || "rename"
        : request.conflictPolicy,
      batchSize: Number(request.batchSize) || 1,
      port: Number(request.port) || 0,
    };
  }

  async function runExport(request: ExportRequest, preview?: ExportPreview) {
    if (isOracleSchemaRequest(request)) {
      await StartTask("export", {
        ...request,
        schema: preview?.schema || request.schema,
        conflictPolicy: request.conflictPolicy || "rename",
      });
      return;
    }

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

  async function clearGeneratedFilesList() {
    await ClearGeneratedFiles();
    generatedFiles = [];
    selectedFiles = [];
    selectionInitialized = true;
    zipEnabled = false;
    zipPassword = "";
    visibility = {
      ...visibility,
      zipPassword: false,
    };
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

  function openConfigDialog() {
    configDraft = { ...form };
    configDialogOpen = true;
  }

  function closeConfigDialog() {
    configDialogOpen = false;
    configDraft = null;
  }

  function onConfigBackdropClick(event: MouseEvent) {
    if (event.target === event.currentTarget) {
      closeConfigDialog();
    }
  }

  function onConfigBackdropKeydown(event: KeyboardEvent) {
    if (event.target !== event.currentTarget) {
      return;
    }

    if (event.key === "Enter" || event.key === " " || event.key === "Escape") {
      event.preventDefault();
      closeConfigDialog();
    }
  }

  function saveConfigDialog() {
    if (!configDraft) {
      closeConfigDialog();
      return;
    }

    form = sanitizeRequest(configDraft);
    closeConfigDialog();
  }

  async function saveConfigAndTest() {
    saveConfigDialog();
    await startTask("test");
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

  function onSchemaConfirmBackdropClick(event: MouseEvent) {
    if (event.target === event.currentTarget) {
      schemaExportConfirmation = null;
    }
  }

  function onSchemaConfirmBackdropKeydown(event: KeyboardEvent) {
    if (event.key === "Enter" || event.key === " " || event.key === "Escape") {
      event.preventDefault();
      schemaExportConfirmation = null;
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

  function replaceOutputDirectory(currentPath: string, schemaName: string) {
    const trimmedSchema = String(schemaName || "").trim() || "schema-export";
    const normalizedPath = String(currentPath || "").trim();
    if (!normalizedPath) {
      return trimmedSchema;
    }

    const separatorIndex = Math.max(
      normalizedPath.lastIndexOf("/"),
      normalizedPath.lastIndexOf("\\"),
    );
    const directory = separatorIndex >= 0 ? normalizedPath.slice(0, separatorIndex + 1) : "";
    const leaf = separatorIndex >= 0 ? normalizedPath.slice(separatorIndex + 1) : normalizedPath;

    if (leaf.toLowerCase().endsWith(".parquet")) {
      return `${directory}${trimmedSchema}`;
    }

    return `${directory}${trimmedSchema}`;
  }

  function isOracleSchemaRequest(request: ExportRequest) {
    return request.backend === "oracle" && request.exportMode === "schema";
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
  <div class:workspace={true} class:is-entering={entering}>
    <header class="topbar">
      <div class="brand-block">
        <p class="eyebrow">Parquet Export Studio</p>
        <h1>导出工作台</h1>
        <p class="subcopy">将数据库表导出为 Parquet 文件。</p>
      </div>

      <div class="hero-actions">
        <button class="button button-secondary" disabled={running} on:click={openConfigDialog}>
          配置数据源
        </button>
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
    </header>

    <section class="hero-strip">
      <div class="hero-status-panel">
        <div class="status-header">
          <span class={`status-dot status-${statusTone}`}></span>
          <span class="status-title">{status}</span>
        </div>
        <p class="status-copy">
          当前数据源 <strong>{backendLabel}</strong>
          {#if connectionSummary}
            ，连接信息 <strong>{connectionSummary}</strong>
          {/if}
          {#if isOracleSchemaMode && form.schema}
            ，目标 Schema <strong>{form.schema}</strong>
          {:else if form.table}
            ，目标表 <strong>{form.table}</strong>
          {/if}
        </p>
        {#if running}
          <div class="status-progress" aria-hidden="true">
            <div class="status-progress-bar"></div>
          </div>
        {/if}
      </div>

      <dl class="hero-stats">
        <div>
          <dt>已写入</dt>
          <dd>{numberWithCommas(rowsWritten)}</dd>
        </div>
        <div>
          <dt>生成文件</dt>
          <dd>{generatedFiles.length}</dd>
        </div>
        <div>
          <dt>日志条数</dt>
          <dd>{logs.length}</dd>
        </div>
      </dl>
    </section>

    <main class="workbench">
      <div class="workbench-main">
        <section class="surface surface-export surface-form">
          <div class="surface-head">
            <div>
              <h2>导出参数</h2>
            </div>
            <p class="surface-note">设置输出路径、批次大小和压缩方式。</p>
          </div>

          {#if isOracleSchemaMode}
            <div class="mode-banner">
              <span class="mode-badge">Schema 模式</span>
              <p>当前将按 Schema 批量导出到目标目录。</p>
            </div>
          {/if}

          <div class="parameter-grid">
            <label class="field field-span-2">
              <span class="field-label"
                >{isOracleSchemaMode ? "输出目录" : "输出 Parquet 路径"}</span
              >
              <span class="field-frame">
                <input bind:value={form.outputPath} />
              </span>
            </label>

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

            <label class="field field-select grow">
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

          <div class="inline-note-row">
            <p class="panel-note">
              {#if isOracleSchemaMode}
                将在输出目录下为 schema 内每张表生成一个独立的 parquet 文件。
              {:else}
                批次越大速度越快，批次越小内存占用越低。
              {/if}
            </p>
            <p class="output-preview">{form.outputPath || "请设置输出路径"}</p>
          </div>
        </section>

        <section class="surface surface-files">
          <div class="surface-head">
            <div>
              <h2>本次生成的 Parquet</h2>
            </div>
            <p class="surface-note">查看本次会话生成的文件，并继续打包。</p>
          </div>

          <div class="generated-toolbar">
            <div class="generated-toolbar-main">
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
            <button
              class="button button-ghost button-inline"
              disabled={generatedFiles.length === 0}
              on:click={clearGeneratedFilesList}
            >
              清空列表
            </button>
          </div>

          <div class="output-section">
            <div class="generated-list" class:is-empty={generatedFiles.length === 0}>
              {#if generatedFiles.length === 0}
                <p class="empty-copy">当前会话还没有导出文件。</p>
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

            <div class="zip-inline">
              <div class="zip-inline-head">
                <div>
                  <h3>ZIP 打包</h3>
                  <p>将选中文件打包为 ZIP。</p>
                </div>
              </div>

              <label class="list-checkbox zip-toggle">
                <input
                  type="checkbox"
                  checked={zipEnabled}
                  disabled={generatedFiles.length === 0}
                  on:change={handleZipEnabledChange}
                />
                <span>启用 ZIP 压缩</span>
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

        <section class="surface surface-log">
          <button
            type="button"
            class="panel-toggle"
            aria-expanded={logsExpanded}
            on:click={() => (logsExpanded = !logsExpanded)}
          >
            <span class="panel-toggle-copy">
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
      </div>
    </main>

    {#if notice}
      <div class={`notice-banner notice-${notice.tone}`} role="status" aria-live="polite">
        <span class="notice-icon" aria-hidden="true">{notice.tone === "success" ? "✓" : "⚠"}</span>
        <span class="notice-message">{notice.message}</span>
      </div>
    {/if}

    {#if configDialogOpen}
      <div
        class="dialog-backdrop"
        role="button"
        tabindex="0"
        aria-label="关闭数据源配置对话框"
        on:click={onConfigBackdropClick}
        on:keydown={onConfigBackdropKeydown}
      >
        <div
          class="dialog-card dialog-card-wide"
          role="dialog"
          aria-modal="true"
          aria-labelledby="configDialogTitle"
        >
          <div class="dialog-head">
            <div>
              <h3 id="configDialogTitle">配置数据源</h3>
              <p class="dialog-copy">设置连接信息与导出目标，保存后返回工作台继续执行。</p>
            </div>
          </div>

          <div class="dialog-form dialog-form-grid surface-form">
            <label class="field field-select grow">
              <span class="field-label">数据源</span>
              <span class="field-frame">
                <select value={dialogForm.backend} on:change={handleBackendChange}>
                  {#each backends as option (option.value)}
                    <option value={option.value}>{option.label}</option>
                  {/each}
                </select>
              </span>
            </label>

            {#if dialogIsOracle}
              <label class="field field-select grow">
                <span class="field-label">导出模式</span>
                <span class="field-frame">
                  <select value={dialogForm.exportMode} on:change={handleExportModeChange}>
                    <option value="table">单表导出</option>
                    <option value="schema">按 Schema 导出全部表</option>
                  </select>
                </span>
              </label>
            {:else}
              <div class="dialog-spacer" aria-hidden="true"></div>
            {/if}

            {#if dialogIsMaxCompute}
              <label class="field field-span-2">
                <span class="field-label">Endpoint</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.maxcomputeEndpoint}
                    on:input={(event) => handleConfigTextInput("maxcomputeEndpoint", event)}
                    placeholder="https://service.cn-hangzhou.maxcompute.aliyun.com/api"
                  />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Access ID</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.maxcomputeAccessId}
                    on:input={(event) => handleConfigTextInput("maxcomputeAccessId", event)}
                  />
                </span>
              </label>

              <label class="field grow field-password">
                <span class="field-label">Access Key</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.maxcomputeAccessKey}
                    on:input={(event) => handleConfigTextInput("maxcomputeAccessKey", event)}
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

              <label class="field">
                <span class="field-label">Project</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.maxcomputeProject}
                    on:input={(event) => handleConfigTextInput("maxcomputeProject", event)}
                  />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Table</span>
                <span class="field-frame">
                  <input value={dialogForm.table} on:input={handleTableInput} />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Schema (optional)</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.schema}
                    on:input={(event) => handleConfigTextInput("schema", event)}
                  />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Partition Spec (optional)</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.partitionSpec}
                    on:input={(event) => handleConfigTextInput("partitionSpec", event)}
                    placeholder="ds='2026-03-07', region='cn'"
                  />
                </span>
              </label>
            {:else}
              <label class="field grow">
                <span class="field-label">Host</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.host}
                    on:input={(event) => handleConfigTextInput("host", event)}
                  />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Port</span>
                <span class="field-frame">
                  <input type="number" value={dialogForm.port} on:input={handlePortInput} />
                </span>
              </label>

              <label class="field field-span-2">
                <span class="field-label">Database / Service</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.database}
                    on:input={(event) => handleConfigTextInput("database", event)}
                  />
                </span>
              </label>

              <label class="field grow">
                <span class="field-label">Username</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.username}
                    on:input={(event) => handleConfigTextInput("username", event)}
                  />
                </span>
              </label>

              <label class="field grow field-password">
                <span class="field-label">Password</span>
                <span class="field-frame">
                  <input
                    value={dialogForm.password}
                    on:input={(event) => handleConfigTextInput("password", event)}
                    type={fieldInputType("password")}
                  />
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

              <label class="field grow">
                <span class="field-label"
                  >{dialogIsOracleSchemaMode ? "Schema" : "Schema (optional)"}</span
                >
                <span class="field-frame">
                  <input value={dialogForm.schema} on:input={handleSchemaInput} />
                </span>
              </label>

              {#if !dialogIsOracleSchemaMode}
                <label class="field grow">
                  <span class="field-label">Table</span>
                  <span class="field-frame">
                    <input value={dialogForm.table} on:input={handleTableInput} />
                  </span>
                </label>
              {:else}
                <div class="dialog-spacer" aria-hidden="true"></div>
              {/if}
            {/if}
          </div>

          <div class="dialog-actions">
            <button class="button button-ghost" on:click={closeConfigDialog}>取消</button>
            <button class="button button-secondary" on:click={saveConfigAndTest}>
              保存并测试
            </button>
            <button class="button button-primary" on:click={saveConfigDialog}>保存配置</button>
          </div>
        </div>
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

    {#if schemaExportConfirmation}
      <div
        class="dialog-backdrop"
        role="button"
        tabindex="0"
        aria-label="关闭 schema 导出确认对话框"
        on:click={onSchemaConfirmBackdropClick}
        on:keydown={onSchemaConfirmBackdropKeydown}
      >
        <div
          class="dialog-card"
          role="dialog"
          aria-modal="true"
          aria-labelledby="schemaExportConfirmTitle"
        >
          <h3 id="schemaExportConfirmTitle">确认批量导出</h3>
          <p class="dialog-copy">
            Schema <strong>{schemaExportConfirmation.schema}</strong> 下共找到
            <strong>{numberWithCommas(schemaExportConfirmation.tableCount)}</strong> 张表，即将全部导出。
          </p>
          <div class="dialog-actions">
            <button class="button button-ghost" on:click={() => (schemaExportConfirmation = null)}
              >取消</button
            >
            <button class="button button-primary" on:click={confirmSchemaExport}>继续导出</button>
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}

<style lang="less">
  .workspace {
    max-width: 1480px;
    margin: 0 auto;
    padding: 22px 24px 32px;
  }

  .topbar {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 20px;
    margin-bottom: 12px;
  }

  .eyebrow {
    margin: 0;
    color: var(--accent-strong);
    font-size: 10.5px;
    font-weight: 800;
    line-height: 1.1;
    letter-spacing: 0.16em;
    text-transform: uppercase;
  }

  .brand-block h1 {
    margin: 4px 0 0;
    font-size: clamp(26px, 3vw, 38px);
    line-height: 0.96;
    letter-spacing: -0.055em;
    font-weight: 800;
  }

  .subcopy {
    max-width: 360px;
    margin: 8px 0 0;
    color: var(--muted);
    font-size: 13px;
    line-height: 1.4;
  }

  .hero-actions {
    display: inline-flex;
    gap: 10px;
    align-items: center;
    flex: 0 0 auto;
    margin-bottom: 2px;
  }

  .hero-strip,
  .surface {
    position: relative;
    overflow: hidden;
    border: 1px solid var(--line);
    border-radius: 24px;
    background: rgba(255, 255, 255, 0.78);
    box-shadow: var(--surface-shadow);
    backdrop-filter: blur(12px);
  }

  .hero-strip {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 14px;
    padding: 20px 22px;
    margin-bottom: 16px;
    background:
      linear-gradient(135deg, rgba(9, 18, 26, 0.95) 0%, rgba(18, 38, 47, 0.88) 70%),
      linear-gradient(180deg, rgba(255, 255, 255, 0.08), transparent);
    border-color: rgba(111, 216, 202, 0.15);
    color: #eef7f5;
    box-shadow: 0 24px 60px rgba(15, 23, 42, 0.22);
  }

  .hero-strip::after {
    content: "";
    position: absolute;
    inset: auto -8% -45% auto;
    width: 340px;
    height: 340px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(98, 227, 211, 0.18), transparent 60%);
    pointer-events: none;
  }

  .hero-status-panel {
    position: relative;
    z-index: 1;
    display: grid;
    gap: 8px;
  }

  .status-header {
    display: inline-flex;
    align-items: center;
    gap: 10px;
  }

  .status-dot {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: #8ea0ae;
    box-shadow: 0 0 0 8px rgba(255, 255, 255, 0.05);
  }

  .status-dot.status-running {
    background: #72e8d7;
    animation: pulse-dot 1.4s ease-in-out infinite;
  }

  .status-dot.status-success {
    background: #7fd8b4;
  }

  .status-dot.status-error {
    background: #f29078;
  }

  .status-title {
    font-size: 18px;
    font-weight: 800;
    letter-spacing: -0.02em;
  }

  .status-copy {
    margin: 0;
    max-width: 520px;
    color: rgba(238, 247, 245, 0.72);
    line-height: 1.42;
  }

  .status-copy strong {
    color: #fff;
    font-weight: 800;
  }

  .status-progress {
    width: min(100%, 420px);
    height: 3px;
    border-radius: 999px;
    overflow: hidden;
    background: rgba(255, 255, 255, 0.12);
  }

  .status-progress-bar {
    width: 32%;
    height: 100%;
    border-radius: inherit;
    background: linear-gradient(90deg, rgba(111, 216, 202, 0.22), rgba(111, 216, 202, 1));
    animation: indeterminate 1.7s ease-in-out infinite;
  }

  .hero-stats {
    position: relative;
    z-index: 1;
    display: grid;
    grid-template-columns: repeat(3, minmax(88px, 1fr));
    gap: 8px;
    margin: 0;
  }

  .hero-stats div {
    min-width: 0;
    padding-left: 16px;
    border-left: 1px solid rgba(255, 255, 255, 0.12);
  }

  .hero-stats dt {
    margin: 0 0 8px;
    color: rgba(238, 247, 245, 0.62);
    font-size: 12px;
    font-weight: 700;
  }

  .hero-stats dd {
    margin: 0;
    color: #fff;
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    font-size: 24px;
    font-weight: 700;
    letter-spacing: -0.04em;
  }

  .workbench {
    display: block;
  }

  .workbench-main {
    display: grid;
    gap: 14px;
  }

  .surface {
    padding: 18px 18px 16px;
    border-radius: 22px;
  }

  .surface::before {
    content: "";
    position: absolute;
    inset: 0 0 auto;
    height: 1px;
    background: linear-gradient(90deg, rgba(79, 151, 145, 0.34), transparent 70%);
    pointer-events: none;
  }

  .surface-head {
    display: flex;
    justify-content: space-between;
    gap: 14px;
    align-items: end;
    margin-bottom: 12px;
  }

  .surface-head h2 {
    margin: 0;
    font-size: 20px;
    line-height: 1;
    letter-spacing: -0.04em;
    font-weight: 800;
  }

  .surface-note {
    margin: 0;
    max-width: 200px;
    color: var(--muted);
    font-size: 12.5px;
    line-height: 1.5;
    text-align: right;
  }

  .surface-form .surface-head {
    align-items: flex-start;
    min-height: 30px;
    margin-bottom: 6px;
  }

  .surface-form .surface-note {
    max-width: 162px;
    padding-top: 1px;
    line-height: 1.45;
  }

  .surface-form
    > :is(label, .field-row, .parameter-grid, .inline-note-row, .mode-banner):not(.surface-head) {
    margin-top: 9px;
  }

  .surface-form
    > .surface-head
    + :is(label, .field-row, .parameter-grid, .inline-note-row, .mode-banner) {
    margin-top: 0;
  }

  .mode-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    padding: 10px 12px;
    margin-bottom: 12px;
    border: 1px solid rgba(60, 139, 132, 0.16);
    border-radius: 14px;
    background: rgba(60, 139, 132, 0.06);
  }

  .mode-banner p {
    margin: 0;
    color: #45606a;
    font-size: 13px;
    line-height: 1.45;
  }

  .mode-badge {
    display: inline-flex;
    align-items: center;
    padding: 6px 10px;
    border-radius: 999px;
    background: rgba(60, 139, 132, 0.12);
    color: var(--accent-strong);
    font-size: 11px;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .parameter-grid,
  .field-row {
    display: grid;
    gap: 10px;
  }

  .parameter-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .field-span-2 {
    grid-column: 1 / -1;
  }

  .schema-row.is-schema-mode {
    grid-template-columns: 1fr;
  }

  .field {
    display: grid;
    gap: 4px;
    margin: 0;
  }

  .grow {
    min-width: 0;
  }

  .field-label {
    color: #5a6777;
    font-size: 11.5px;
    font-weight: 800;
    line-height: 1.2;
    letter-spacing: 0.01em;
    padding-left: 2px;
    min-height: 14px;
  }

  .field-frame {
    position: relative;
    display: block;
    padding: 9px 12px 8px;
    min-height: 46px;
    border: 1px solid var(--field-border);
    border-radius: 13px;
    background: var(--field-bg);
    transition:
      border-color 0.16s ease,
      box-shadow 0.16s ease,
      background-color 0.16s ease,
      transform 0.16s ease;
  }

  .field:focus-within .field-frame {
    border-color: rgba(60, 139, 132, 0.54);
    box-shadow: 0 0 0 4px rgba(60, 139, 132, 0.08);
    background: #fff;
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
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ink);
    outline: none;
    appearance: none;
  }

  input::placeholder,
  textarea::placeholder {
    color: #97a3b2;
  }

  select {
    padding-right: 26px;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8' fill='none'%3E%3Cpath d='M1 1.5L6 6.5L11 1.5' stroke='%236a7283' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right center;
    background-size: 12px 8px;
  }

  .field-password input {
    padding-right: 28px;
  }

  .field-select select,
  input {
    line-height: 1.25;
  }

  .field-visibility-toggle {
    position: absolute;
    right: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 28px;
    height: 28px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: #8190a1;
    cursor: pointer;
  }

  .field-visibility-toggle:hover {
    color: var(--accent-strong);
    background: rgba(79, 151, 145, 0.08);
  }

  .field-visibility-toggle:focus-visible {
    outline: 2px solid rgba(79, 151, 145, 0.24);
    outline-offset: 2px;
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

  .inline-note-row {
    display: flex;
    justify-content: space-between;
    gap: 14px;
    align-items: center;
    margin-top: 10px;
  }

  .panel-note,
  .output-preview,
  .generated-meta,
  .generated-file-meta,
  .zip-note,
  .empty-copy {
    color: var(--muted);
    font-size: 13px;
  }

  .panel-note,
  .zip-note,
  .empty-copy {
    margin: 0;
    line-height: 1.5;
  }

  .output-preview {
    margin: 0;
    max-width: 50%;
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    text-align: right;
    overflow-wrap: anywhere;
  }

  .output-section {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(300px, 340px);
    gap: 16px;
    align-items: start;
    margin-top: 12px;
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
    padding-bottom: 10px;
    border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  }

  .generated-toolbar-main {
    display: inline-flex;
    align-items: center;
    gap: 14px;
    min-width: 0;
  }

  .generated-list {
    display: grid;
    gap: 6px;
    margin-top: 10px;
  }

  .generated-list.is-empty {
    min-height: 132px;
    padding: 16px 18px;
    align-content: center;
    border: 1px dashed rgba(148, 163, 184, 0.24);
    border-radius: 16px;
    background: rgba(245, 248, 250, 0.62);
  }

  .generated-item {
    padding: 10px 0;
    border-bottom: 1px solid rgba(148, 163, 184, 0.14);
    transition:
      transform 0.18s ease,
      color 0.18s ease;
  }

  .generated-item:last-child {
    border-bottom: none;
  }

  .generated-item:hover {
    transform: translateX(2px);
  }

  .generated-item.is-selected .generated-file-copy strong,
  .generated-item.is-selected .generated-file-meta {
    color: var(--accent-strong);
  }

  .list-checkbox {
    display: inline-flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }

  .list-checkbox input[type="checkbox"] {
    width: 20px;
    height: 20px;
    min-width: 20px;
    margin: 0;
    border: 1px solid rgba(113, 129, 151, 0.54);
    border-radius: 6px;
    background: #fff;
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
    border-color: rgba(60, 139, 132, 0.62);
    box-shadow: 0 0 0 4px rgba(60, 139, 132, 0.08);
  }

  .list-checkbox input[type="checkbox"]:focus-visible {
    outline: none;
    border-color: rgba(60, 139, 132, 0.72);
    box-shadow: 0 0 0 4px rgba(60, 139, 132, 0.1);
  }

  .list-checkbox input[type="checkbox"]:checked {
    border-color: var(--accent-strong);
    background: var(--accent-strong);
  }

  .list-checkbox input[type="checkbox"]:checked::after {
    content: "";
    position: absolute;
    left: 6px;
    top: 2px;
    width: 4px;
    height: 9px;
    border: solid #fff;
    border-width: 0 2px 2px 0;
    transform: rotate(45deg);
  }

  .list-checkbox input[type="checkbox"]:disabled {
    cursor: not-allowed;
    opacity: 0.45;
  }

  .master-checkbox span:last-child,
  .zip-toggle span:last-child {
    font-weight: 700;
  }

  .generated-file-copy {
    min-width: 0;
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
    flex: 0 0 auto;
    white-space: nowrap;
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
  }

  .zip-inline {
    display: grid;
    align-content: start;
    gap: 12px;
    padding-left: 16px;
    border-left: 1px solid rgba(148, 163, 184, 0.16);
  }

  .zip-inline-head h3 {
    margin: 0;
    font-size: 17px;
    letter-spacing: -0.03em;
  }

  .zip-inline-head p {
    margin: 4px 0 0;
    color: var(--muted);
    font-size: 13px;
    line-height: 1.5;
  }

  .zip-toggle {
    margin-top: 0;
  }

  .compact-field {
    margin-top: 0;
  }

  .zip-actions {
    align-items: flex-end;
    margin-top: 0;
  }

  .zip-note {
    max-width: 180px;
    text-align: right;
  }

  .surface-log {
    padding-top: 14px;
  }

  .panel-toggle {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    gap: 12px;
    padding: 0;
    border: none;
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .panel-toggle-copy {
    display: grid;
    gap: 6px;
  }

  .panel-toggle-title {
    font-size: 20px;
    font-weight: 800;
    line-height: 1;
    letter-spacing: -0.04em;
  }

  .panel-toggle-meta {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    color: var(--muted);
  }

  .panel-toggle-state {
    font-size: 12px;
    font-weight: 700;
  }

  .panel-chevron {
    transition: transform 0.25s cubic-bezier(0.22, 1, 0.36, 1);
  }

  .panel-chevron.is-open {
    transform: rotate(180deg);
  }

  .log-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 24px;
    height: 24px;
    padding: 0 8px;
    border-radius: 999px;
    background: rgba(60, 139, 132, 0.1);
    color: var(--accent-strong);
    font-size: 11px;
    font-weight: 800;
  }

  .log-collapse {
    max-height: 0;
    overflow: hidden;
    transition:
      max-height 0.28s cubic-bezier(0.22, 1, 0.36, 1),
      margin-top 0.28s ease;
  }

  .log-collapse.is-open {
    max-height: 420px;
    margin-top: 16px;
  }

  textarea {
    min-height: 220px;
    resize: vertical;
    padding: 13px 14px;
    border: 1px solid var(--field-border);
    border-radius: 16px;
    background: rgba(247, 249, 251, 0.9);
    font-family: "SF Mono", "JetBrains Mono", "Menlo", monospace;
    font-size: 12.5px;
    line-height: 1.55;
  }

  textarea:focus {
    border-color: rgba(60, 139, 132, 0.56);
    box-shadow: 0 0 0 4px rgba(60, 139, 132, 0.08);
    background: #fff;
  }

  .button {
    border: none;
    border-radius: 12px;
    padding: 11px 18px;
    cursor: pointer;
    font-size: 13.5px;
    font-weight: 800;
    letter-spacing: -0.01em;
    transition:
      transform 0.14s ease,
      background-color 0.14s ease,
      border-color 0.14s ease,
      box-shadow 0.14s ease,
      opacity 0.14s ease;
  }

  .button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .button:hover:not(:disabled) {
    transform: translateY(-1px);
  }

  .button-icon {
    margin-right: 5px;
  }

  .button-primary {
    background: linear-gradient(180deg, #56aaa0 0%, #3c8b84 100%);
    color: #fff;
    box-shadow: 0 12px 24px rgba(60, 139, 132, 0.24);
  }

  .surface-files {
    background: linear-gradient(
      180deg,
      rgba(255, 255, 255, 0.82) 0%,
      rgba(252, 253, 253, 0.74) 100%
    );
  }

  .button-secondary {
    border: 1px solid rgba(19, 35, 63, 0.12);
    background: rgba(255, 255, 255, 0.78);
    color: var(--ink);
  }

  .button-ghost {
    background: rgba(148, 163, 184, 0.14);
    color: var(--ink);
  }

  .button-inline {
    padding: 8px 12px;
    font-size: 12.5px;
    font-weight: 700;
  }

  .notice-banner {
    position: fixed;
    left: 50%;
    bottom: 24px;
    z-index: 1000;
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: min(420px, calc(100vw - 32px));
    max-width: calc(100vw - 32px);
    padding: 12px 16px;
    border-radius: 12px;
    box-shadow: 0 18px 40px rgba(15, 23, 42, 0.18);
    color: #fff;
    animation: notice-in 0.28s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
  }

  .notice-error {
    background: linear-gradient(135deg, #d65d48 0%, #c24133 100%);
  }

  .notice-success {
    background: linear-gradient(135deg, #3c8b84 0%, #2f6f69 100%);
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
    overflow: auto;
    background: rgba(15, 23, 42, 0.28);
    backdrop-filter: blur(10px);
  }

  .dialog-card {
    width: min(560px, 100%);
    padding: 24px;
    border: 1px solid var(--line);
    border-radius: 24px;
    background: rgba(251, 252, 253, 0.96);
    box-shadow: 0 28px 60px rgba(15, 23, 42, 0.22);
  }

  .dialog-card-wide {
    width: min(820px, 100%);
    max-height: min(820px, calc(100vh - 48px));
    display: flex;
    flex-direction: column;
    padding: 22px 22px 20px;
  }

  .dialog-card h3 {
    margin: 0 0 10px;
    font-size: 22px;
    font-weight: 800;
    letter-spacing: -0.03em;
  }

  .dialog-head {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    align-items: end;
    margin-bottom: 16px;
  }

  .dialog-copy {
    margin: 0;
    color: var(--muted);
    line-height: 1.55;
  }

  .dialog-form {
    display: grid;
    gap: 10px;
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: 16px;
    border: 1px solid rgba(148, 163, 184, 0.16);
    border-radius: 18px;
    background:
      linear-gradient(180deg, rgba(248, 251, 252, 0.96), rgba(255, 255, 255, 0.9)),
      rgba(255, 255, 255, 0.76);
  }

  .dialog-form-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    align-items: start;
  }

  .dialog-spacer {
    min-height: 0;
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
    background: rgba(255, 255, 255, 0.74);
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
    flex: 0 0 auto;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 18px;
  }

  .fatal-error {
    max-width: 960px;
    margin: 40px auto;
    white-space: pre-wrap;
  }

  .is-entering .hero-strip,
  .is-entering .surface {
    animation: panel-in 0.42s cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  .is-entering .surface-export {
    animation-delay: 0.05s;
  }

  .is-entering .surface-files {
    animation-delay: 0.1s;
  }

  .is-entering .surface-log {
    animation-delay: 0.15s;
  }

  .is-entering .generated-item {
    animation: item-in 0.28s ease both;
  }

  @keyframes pulse-dot {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }

    50% {
      opacity: 0.45;
      transform: scale(0.82);
    }
  }

  @keyframes indeterminate {
    0% {
      transform: translateX(-100%);
    }

    100% {
      transform: translateX(430%);
    }
  }

  @keyframes notice-in {
    from {
      transform: translateX(-50%) translateY(22px);
      opacity: 0;
    }

    to {
      transform: translateX(-50%) translateY(0);
      opacity: 1;
    }
  }

  @keyframes panel-in {
    from {
      opacity: 0;
      transform: translateY(14px);
    }

    to {
      opacity: 1;
      transform: translateY(0);
    }
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

  @media (max-width: 1220px) {
    .hero-strip {
      grid-template-columns: 1fr;
    }

    .hero-stats {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }

    .surface-note {
      max-width: none;
      text-align: left;
    }

    .output-section {
      grid-template-columns: 1fr;
      gap: 18px;
    }

    .zip-inline {
      padding-left: 0;
      padding-top: 18px;
      border-left: none;
      border-top: 1px solid rgba(148, 163, 184, 0.16);
    }
  }

  @media (max-width: 820px) {
    .workspace {
      padding: 16px 16px 28px;
    }

    .topbar,
    .surface-head,
    .inline-note-row,
    .zip-actions,
    .generated-item,
    .dialog-actions,
    .dialog-head {
      display: grid;
    }

    .generated-toolbar {
      gap: 10px;
    }

    .generated-toolbar-main {
      justify-content: space-between;
    }

    .hero-actions {
      width: 100%;
      display: grid;
      grid-template-columns: 1fr;
    }

    .parameter-grid,
    .field-row,
    .hero-stats,
    .dialog-form-grid {
      grid-template-columns: 1fr;
    }

    .dialog-spacer {
      display: none;
    }

    .mode-banner,
    .output-preview,
    .zip-note {
      max-width: none;
      text-align: left;
    }

    .hero-actions .button {
      width: 100%;
    }

    .notice-banner {
      left: 16px;
      right: 16px;
      bottom: 16px;
      transform: none;
      min-width: 0;
    }

    .dialog-card-wide {
      width: min(100%, 820px);
      max-height: calc(100vh - 24px);
      padding: 18px;
    }
  }
</style>
