package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Backend              string `json:"backend"`
	OutputPath           string `json:"outputPath"`
	Table                string `json:"table"`
	BatchSize            int    `json:"batchSize"`
	Compression          string `json:"compression"`
	Schema               string `json:"schema"`
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Username             string `json:"username"`
	Password             string `json:"password"`
	Database             string `json:"database"`
	MaxComputeEndpoint   string `json:"maxcomputeEndpoint"`
	MaxComputeProject    string `json:"maxcomputeProject"`
	MaxComputeAccessID   string `json:"maxcomputeAccessId"`
	MaxComputeAccessKey  string `json:"maxcomputeAccessKey"`
	PartitionSpec        string `json:"partitionSpec"`
}

type AppConfig struct {
	Backends     []Option          `json:"backends"`
	Compressions []string          `json:"compressions"`
	DefaultPorts map[string]int    `json:"defaultPorts"`
	DefaultState ExportRequest     `json:"defaultState"`
	BackendHints map[string]string `json:"backendHints"`
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
			"maxcompute": 443,
		},
		DefaultState: ExportRequest{
			Backend:     "oracle",
			OutputPath:  filepath.Join(userHomeDir(), "Downloads", "export.parquet"),
			BatchSize:   5000,
			Compression: "snappy",
			Port:        1521,
		},
		BackendHints: map[string]string{
			"oracle":     "Oracle 现在改用 Go 原生驱动直连，不再依赖 Python 运行时。",
			"mysql":      "MySQL 采用 Go 原生流式查询和 Arrow/Parquet 写入。",
			"postgresql": "PostgreSQL 采用 Go 原生驱动导出，整个链路已不再依赖外部解释器。",
			"maxcompute": "MaxCompute 已恢复为 Go 原生接入，使用官方 ODPS SQL driver 直连。",
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

		if err := a.runTask(kind, request); err != nil {
			a.emit(TaskEvent{Kind: kind, Type: "error", Error: err.Error()})
		}
	}()

	return nil
}

func (a *App) emit(event TaskEvent) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "task:event", event)
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}
