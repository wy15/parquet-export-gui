package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	mu      sync.Mutex
	running bool
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type ExportRequest struct {
	Backend             string `json:"backend"`
	OutputPath          string `json:"outputPath"`
	Table               string `json:"table"`
	BatchSize           int    `json:"batchSize"`
	Compression         string `json:"compression"`
	Schema              string `json:"schema"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	Database            string `json:"database"`
	MaxComputeEndpoint  string `json:"maxcomputeEndpoint"`
	MaxComputeProject   string `json:"maxcomputeProject"`
	MaxComputeAccessID  string `json:"maxcomputeAccessId"`
	MaxComputeAccessKey string `json:"maxcomputeAccessKey"`
	PartitionSpec       string `json:"partitionSpec"`
}

type AppConfig struct {
	Backends     []Option          `json:"backends"`
	Compressions []string          `json:"compressions"`
	DefaultPorts map[string]int    `json:"defaultPorts"`
	DefaultState ExportRequest     `json:"defaultState"`
	BackendHints map[string]string `json:"backendHints"`
}

type bridgeEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type TaskEvent struct {
	Kind        string `json:"kind"`
	Type        string `json:"type"`
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
	OutputPath  string `json:"outputPath,omitempty"`
	RowsWritten int    `json:"rowsWritten,omitempty"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetConfig() AppConfig {
	return AppConfig{
		Backends: []Option{
			{Value: "oracle", Label: "Oracle (Thin)"},
			{Value: "mysql", Label: "MySQL"},
			{Value: "postgresql", Label: "PostgreSQL"},
			{Value: "maxcompute", Label: "MaxCompute"},
		},
		Compressions: []string{"snappy", "gzip", "brotli", "lz4", "zstd", "none"},
		DefaultPorts: map[string]int{
			"oracle":     1521,
			"mysql":      3306,
			"postgresql": 5432,
		},
		DefaultState: ExportRequest{
			Backend:     "oracle",
			OutputPath:  filepath.Join(userHomeDir(), "Downloads", "export.parquet"),
			BatchSize:   5000,
			Compression: "snappy",
			Port:        1521,
		},
		BackendHints: map[string]string{
			"oracle":     "Oracle 默认使用 python-oracledb Thin 模式，填写 Service Name 即可。",
			"mysql":      "MySQL 采用服务端游标流式读取，适合大表导出。",
			"postgresql": "PostgreSQL 使用 named cursor 分批拉取结果，避免客户端一次性缓存。",
			"maxcompute": "MaxCompute 通过表读取接口导出 Arrow RecordBatch，再写入本地 Parquet。",
		},
	}
}

func (a *App) StartTask(kind string, request ExportRequest) error {
	if kind != "test" && kind != "export" {
		return fmt.Errorf("unsupported task kind: %s", kind)
	}

	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("已有任务正在运行")
	}
	a.running = true
	a.mu.Unlock()

	a.emit(TaskEvent{Kind: kind, Type: "running", Message: "true"})

	go func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			a.emit(TaskEvent{Kind: kind, Type: "running", Message: "false"})
		}()

		if err := a.runBridge(kind, request); err != nil {
			a.emit(TaskEvent{Kind: kind, Type: "error", Error: err.Error()})
		}
	}()

	return nil
}

func (a *App) runBridge(kind string, request ExportRequest) error {
	root, err := findProjectRoot()
	if err != nil {
		return err
	}

	pythonBin, err := resolvePythonBin(root)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	cmd := exec.Command(pythonBin, "-m", "parquet_export_gui.bridge", kind)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(root, "src"))
	cmd.Stdin = bytes.NewReader(payload)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- a.consumeBridgeOutput(kind, stdout)
	}()
	go func() {
		var stderrBuf bytes.Buffer
		if _, readErr := stderrBuf.ReadFrom(stderr); readErr != nil {
			errCh <- readErr
			return
		}
		text := strings.TrimSpace(stderrBuf.String())
		if text != "" {
			errCh <- errors.New(text)
			return
		}
		errCh <- nil
	}()

	var firstErr error
	for i := 0; i < 2; i++ {
		if pipeErr := <-errCh; pipeErr != nil && firstErr == nil {
			firstErr = pipeErr
		}
	}

	waitErr := cmd.Wait()
	if waitErr != nil && firstErr == nil {
		firstErr = waitErr
	}

	return firstErr
}

func (a *App) consumeBridgeOutput(kind string, stdout io.Reader) error {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var envelope bridgeEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			return err
		}

		switch envelope.Type {
		case "log":
			var message string
			if err := json.Unmarshal(envelope.Payload, &message); err != nil {
				return err
			}
			a.emit(TaskEvent{Kind: kind, Type: "log", Message: message})
		case "success":
			var result struct {
				OutputPath  string `json:"output_path"`
				RowsWritten int    `json:"rows_written"`
			}
			if err := json.Unmarshal(envelope.Payload, &result); err != nil {
				return err
			}
			a.emit(TaskEvent{
				Kind:        kind,
				Type:        "success",
				OutputPath:  result.OutputPath,
				RowsWritten: result.RowsWritten,
			})
		case "error":
			var message string
			if err := json.Unmarshal(envelope.Payload, &message); err != nil {
				return err
			}
			a.emit(TaskEvent{Kind: kind, Type: "error", Error: message})
		default:
			return fmt.Errorf("unsupported bridge event: %s", envelope.Type)
		}
	}

	return scanner.Err()
}

func (a *App) emit(event TaskEvent) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "task:event", event)
}

func findProjectRoot() (string, error) {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir), filepath.Join(filepath.Dir(exeDir), "Resources"))
	}

	for _, candidate := range candidates {
		for _, dir := range ancestorDirs(candidate) {
			bridgePath := filepath.Join(dir, "src", "parquet_export_gui", "bridge.py")
			if fileExists(bridgePath) {
				return dir, nil
			}
		}
	}

	return "", errors.New("未找到 Python bridge，请在仓库根目录运行或设置正确的工作目录")
}

func resolvePythonBin(root string) (string, error) {
	candidates := []string{}
	if env := os.Getenv("PYTHON_BIN"); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates, pythonCandidates(root)...)

	if wd, err := os.Getwd(); err == nil {
		for _, dir := range ancestorDirs(wd) {
			candidates = append(candidates, pythonCandidates(dir)...)
		}
	}
	if exe, err := os.Executable(); err == nil {
		for _, dir := range ancestorDirs(filepath.Dir(exe)) {
			candidates = append(candidates, pythonCandidates(dir)...)
		}
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}

	if python3, err := exec.LookPath("python3"); err == nil {
		return python3, nil
	}

	return "", errors.New("未找到可用的 Python，请先创建 .venv 或设置 PYTHON_BIN")
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func ancestorDirs(start string) []string {
	if start == "" {
		return nil
	}

	dir := filepath.Clean(start)
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	var result []string
	for {
		result = append(result, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return result
}

func pythonCandidates(root string) []string {
	if root == "" {
		return nil
	}
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(root, ".venv", "Scripts", "python.exe"),
			filepath.Join(root, ".venv", "python.exe"),
		}
	}
	return []string{
		filepath.Join(root, ".venv", "bin", "python"),
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
