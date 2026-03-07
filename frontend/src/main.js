import './style.css';
import './app.css';

import { EventsOn } from '../wailsjs/runtime/runtime';
import { GetConfig, StartTask } from '../wailsjs/go/main/App';

const appRoot = document.querySelector('#app');

const state = {
  backends: [],
  compressions: [],
  defaultPorts: {},
  backendHints: {},
  running: false,
  rowsWritten: 0,
  status: '待命',
  logs: [],
  form: {
    backend: 'oracle',
    outputPath: '',
    table: '',
    batchSize: 5000,
    compression: 'snappy',
    schema: '',
    host: '',
    port: 1521,
    username: '',
    password: '',
    database: '',
    maxcomputeEndpoint: '',
    maxcomputeProject: '',
    maxcomputeAccessId: '',
    maxcomputeAccessKey: '',
    partitionSpec: '',
  },
};

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
            <label class="field">
              <span>数据源</span>
              <select id="backend">${renderBackendOptions()}</select>
            </label>

            <div class="${state.form.backend === 'maxcompute' ? 'hidden' : ''}">
              <label class="field">
                <span>Host</span>
                <input id="host" value="${escapeAttr(state.form.host)}" />
              </label>
              <label class="field">
                <span>Port</span>
                <input id="port" value="${escapeAttr(String(state.form.port || ''))}" />
              </label>
              <div class="field-row">
                <label class="field grow">
                  <span>Username</span>
                  <input id="username" value="${escapeAttr(state.form.username)}" />
                </label>
                <label class="field grow">
                  <span>Password</span>
                  <input id="password" type="password" value="${escapeAttr(state.form.password)}" />
                </label>
              </div>
              <label class="field">
                <span>Database / Service Name</span>
                <input id="database" value="${escapeAttr(state.form.database)}" />
              </label>
            </div>

            <div class="${state.form.backend === 'maxcompute' ? '' : 'hidden'}">
              <label class="field">
                <span>Endpoint</span>
                <input id="maxcomputeEndpoint" value="${escapeAttr(state.form.maxcomputeEndpoint)}" />
              </label>
              <label class="field">
                <span>Project</span>
                <input id="maxcomputeProject" value="${escapeAttr(state.form.maxcomputeProject)}" />
              </label>
              <label class="field">
                <span>Access ID</span>
                <input id="maxcomputeAccessId" value="${escapeAttr(state.form.maxcomputeAccessId)}" />
              </label>
              <label class="field">
                <span>Access Key</span>
                <input id="maxcomputeAccessKey" type="password" value="${escapeAttr(state.form.maxcomputeAccessKey)}" />
              </label>
            </div>

            <div class="field-row">
              <label class="field grow">
                <span>Schema (optional)</span>
                <input id="schema" value="${escapeAttr(state.form.schema)}" />
              </label>
              <label class="field grow">
                <span>Table</span>
                <input id="table" value="${escapeAttr(state.form.table)}" />
              </label>
            </div>

            <label class="field ${state.form.backend === 'maxcompute' ? '' : 'hidden'}">
              <span>Partition Spec</span>
              <input id="partitionSpec" value="${escapeAttr(state.form.partitionSpec)}" placeholder="ds=20260306,region=cn" />
            </label>
          </section>

          <div class="panel-stack">
            <section class="panel">
              <h2>导出参数</h2>
              <label class="field">
                <span>输出 Parquet 路径</span>
                <input id="outputPath" value="${escapeAttr(state.form.outputPath)}" />
              </label>
              <div class="field-row">
                <label class="field grow">
                  <span>批次大小</span>
                  <input id="batchSize" type="number" min="1" step="1000" value="${escapeAttr(String(state.form.batchSize))}" />
                </label>
                <label class="field grow">
                  <span>压缩</span>
                  <select id="compression">${renderCompressionOptions()}</select>
                </label>
              </div>
              <p class="panel-note">当前版本先使用 Wails 重构桌面壳，连接与导出仍通过 Python bridge 复用既有实现。</p>
              <div class="button-row">
                <button id="testButton" class="button button-secondary" ${state.running ? 'disabled' : ''}>测试连接</button>
                <button id="exportButton" class="button button-primary" ${state.running ? 'disabled' : ''}>开始导出</button>
              </div>
            </section>

            <section class="panel">
              <h2>运行日志</h2>
              <textarea id="logView" readonly placeholder="日志会显示在这里">${escapeHtml(state.logs.join('\n'))}</textarea>
            </section>
          </div>
        </div>
      </section>
    </div>
  `;

  bindInputs();
}

function bindInputs() {
  document.querySelector('#backend').addEventListener('change', (event) => {
    const backend = event.target.value;
    state.form.backend = backend;
    if (state.defaultPorts[backend]) {
      state.form.port = state.defaultPorts[backend];
    }
    render();
  });

  [
    ['host', 'host'],
    ['port', 'port'],
    ['username', 'username'],
    ['password', 'password'],
    ['database', 'database'],
    ['schema', 'schema'],
    ['table', 'table'],
    ['outputPath', 'outputPath'],
    ['batchSize', 'batchSize'],
    ['compression', 'compression'],
    ['maxcomputeEndpoint', 'maxcomputeEndpoint'],
    ['maxcomputeProject', 'maxcomputeProject'],
    ['maxcomputeAccessId', 'maxcomputeAccessId'],
    ['maxcomputeAccessKey', 'maxcomputeAccessKey'],
    ['partitionSpec', 'partitionSpec'],
  ].forEach(([id, key]) => {
    const element = document.querySelector(`#${id}`);
    if (!element) {
      return;
    }
    element.addEventListener('input', (event) => {
      const value = event.target.value;
      if (key === 'port' || key === 'batchSize') {
        state.form[key] = value === '' ? 0 : Number(value);
      } else {
        state.form[key] = value;
      }
    });
  });

  document.querySelector('#testButton').addEventListener('click', () => startTask('test'));
  document.querySelector('#exportButton').addEventListener('click', () => startTask('export'));
}

async function startTask(kind) {
  try {
    await StartTask(kind, sanitizeRequest());
  } catch (error) {
    appendLog(`ERROR: ${error}`);
    state.status = '执行失败';
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
  EventsOn('task:event', (event) => {
    switch (event.type) {
      case 'running':
        state.running = event.message === 'true';
        if (state.running) {
          state.status = event.kind === 'export' ? '导出中' : '测试中';
          if (event.kind === 'export') {
            state.rowsWritten = 0;
          }
        } else if (state.status === '导出中' || state.status === '测试中') {
          state.status = '待命';
        }
        break;
      case 'log':
        appendLog(event.message);
        break;
      case 'success':
        if (event.kind === 'export') {
          state.status = '导出完成';
          state.rowsWritten = event.rowsWritten;
          appendLog(`导出成功: ${event.outputPath}`);
        } else {
          state.status = '连接成功';
          appendLog('连接成功');
        }
        break;
      case 'error':
        state.status = '执行失败';
        appendLog(`ERROR: ${event.error}`);
        break;
      default:
        appendLog(`WARN: 未知事件 ${event.type}`);
        break;
    }
    render();
  });
}

function backendHint() {
  return state.backendHints[state.form.backend] || '';
}

function renderBackendOptions() {
  return state.backends
    .map((option) => `<option value="${option.value}" ${option.value === state.form.backend ? 'selected' : ''}>${option.label}</option>`)
    .join('');
}

function renderCompressionOptions() {
  return state.compressions
    .map((option) => `<option value="${option}" ${option === state.form.compression ? 'selected' : ''}>${option}</option>`)
    .join('');
}

function appendLog(message) {
  const timestamp = new Date().toLocaleTimeString('zh-CN', { hour12: false });
  state.logs.push(`[${timestamp}] ${message}`);
  state.logs = state.logs.slice(-300);
}

function numberWithCommas(value) {
  return new Intl.NumberFormat('zh-CN').format(value || 0);
}

function escapeAttr(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

async function bootstrap() {
  const config = await GetConfig();
  state.backends = config.backends;
  state.compressions = config.compressions;
  state.defaultPorts = config.defaultPorts;
  state.backendHints = config.backendHints;
  state.form = { ...state.form, ...config.defaultState };
  installEventBridge();
  render();
}

bootstrap().catch((error) => {
  appRoot.innerHTML = `<pre class="fatal-error">${escapeHtml(String(error))}</pre>`;
});
