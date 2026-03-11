package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type App struct {
	mu             sync.Mutex
	running        bool
	generatedFiles []GeneratedFile
	emitFunc       func(TaskEvent)
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type ExportRequest struct {
	Backend             string `json:"backend"`
	OutputPath          string `json:"outputPath"`
	ConflictPolicy      string `json:"conflictPolicy"`
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

type ExportOutputCheck struct {
	Exists        bool   `json:"exists"`
	ResolvedPath  string `json:"resolvedPath"`
	SuggestedPath string `json:"suggestedPath"`
}

type GeneratedFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"createdAt"`
}

type ZipRequest struct {
	Files    []string `json:"files"`
	Password string   `json:"password"`
}

type ZipResult struct {
	OutputPath string `json:"outputPath"`
	FileCount  int    `json:"fileCount"`
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
			Compression: "zstd",
			Port:        1521,
		},
		BackendHints: map[string]string{
			"oracle":     "Oracle 采用 Go 原生驱动直连。",
			"mysql":      "MySQL 采用 Go 原生流式查询和 Arrow/Parquet 写入。",
			"postgresql": "PostgreSQL 采用 Go 原生驱动导出，整个链路已不再依赖外部解释器。",
			"maxcompute": "MaxCompute 采用 Go 原生接入，使用官方 ODPS SQL driver 直连。",
		},
	}
}

func (a *App) StartTask(kind string, request ExportRequest) error {
	return a.startTask(kind, request, true)
}

func (a *App) RunTaskSync(kind string, request ExportRequest) error {
	return a.startTask(kind, request, false)
}

func (a *App) startTask(kind string, request ExportRequest, async bool) error {
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

	run := func() error {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			a.emit(TaskEvent{Kind: kind, Type: "running", Message: "false"})
		}()

		if err := a.runTask(kind, request); err != nil {
			a.emit(TaskEvent{Kind: kind, Type: "error", Error: err.Error()})
			return err
		}
		return nil
	}

	if async {
		go func() {
			_ = run()
		}()
		return nil
	}

	return run()
}

func (a *App) GetGeneratedFiles() []GeneratedFile {
	a.mu.Lock()
	defer a.mu.Unlock()

	files := make([]GeneratedFile, len(a.generatedFiles))
	for i, file := range a.generatedFiles {
		if info, err := os.Stat(file.Path); err == nil {
			file.Size = info.Size()
			file.CreatedAt = info.ModTime().Format(time.RFC3339)
			a.generatedFiles[i] = file
		}
		files[i] = file
	}
	return files
}

func (a *App) CreateZipArchive(request ZipRequest) (ZipResult, error) {
	files, err := a.validateZipRequest(request)
	if err != nil {
		return ZipResult{}, err
	}

	outputPath := buildZipOutputPath(files[0])
	if err := writeZipArchive(outputPath, files, request.Password); err != nil {
		return ZipResult{}, err
	}

	return ZipResult{
		OutputPath: outputPath,
		FileCount:  len(files),
	}, nil
}

func (a *App) CheckExportOutput(path string) (ExportOutputCheck, error) {
	resolvedPath, err := resolveOutputPath(path)
	if err != nil {
		return ExportOutputCheck{}, err
	}

	exists, err := fileExists(resolvedPath)
	if err != nil {
		return ExportOutputCheck{}, err
	}

	suggestedPath := resolvedPath
	if exists {
		suggestedPath, err = nextAvailablePath(resolvedPath)
		if err != nil {
			return ExportOutputCheck{}, err
		}
	}

	return ExportOutputCheck{
		Exists:        exists,
		ResolvedPath:  resolvedPath,
		SuggestedPath: suggestedPath,
	}, nil
}

func (a *App) emit(event TaskEvent) {
	if a.emitFunc != nil {
		a.emitFunc(event)
	}
}

func (a *App) SetEmitter(fn func(TaskEvent)) {
	a.emitFunc = fn
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func (a *App) rememberGeneratedFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	file := GeneratedFile{
		Path:      path,
		Name:      filepath.Base(path),
		Size:      info.Size(),
		CreatedAt: info.ModTime().Format(time.RFC3339),
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	filtered := make([]GeneratedFile, 0, len(a.generatedFiles)+1)
	filtered = append(filtered, file)
	for _, item := range a.generatedFiles {
		if item.Path == path {
			continue
		}
		filtered = append(filtered, item)
	}
	a.generatedFiles = filtered
	return nil
}
