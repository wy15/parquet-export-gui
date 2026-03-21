package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"parquet-export-gui/internal/appcore"
	"path/filepath"
	"strconv"
	"strings"
)

var errExportCancelled = errors.New("export cancelled")

func main() {
	app := appcore.NewApp()
	app.SetEmitter(printCLIEvent)

	if err := runInteractiveCLI(app); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func runInteractiveCLI(app *appcore.App) error {
	reader := bufio.NewReader(os.Stdin)
	config := app.GetConfig()
	request := config.DefaultState

	fmt.Println("Parquet Export Studio CLI")
	fmt.Println("适用于低版本 Windows 的交互式命令行导出工具")
	fmt.Println()

	request.Backend = promptSelectBackend(reader, config.Backends, request.Backend)
	if port, ok := config.DefaultPorts[request.Backend]; ok {
		request.Port = port
	}

	if request.Backend == "maxcompute" {
		request.MaxComputeEndpoint = promptString(reader, "MaxCompute Endpoint", "http://service.cn-jilin-jlyb-d01.odps.ops.jl.hsip.gov.cn/api")
		request.MaxComputeProject = promptString(reader, "Project", "")
		request.MaxComputeAccessID = promptString(reader, "Access ID", "")
		request.MaxComputeAccessKey = promptString(reader, "Access Key", "")
		request.Schema = promptString(reader, "Schema（可选）", "")
		request.Table = promptString(reader, "Table", "")
		request.PartitionSpec = promptString(reader, "Partition Spec（可选）", "")
	} else {
		if request.Backend == "oracle" {
			request.ExportMode = promptSelect(reader, "导出模式", []menuOption{
				{Value: "table", Label: "单表导出"},
				{Value: "schema", Label: "按 Schema 导出全部表"},
			}, request.ExportMode)
		} else {
			request.ExportMode = "table"
		}

		request.Host = promptString(reader, "Host", "127.0.0.1")
		request.Port = promptInt(reader, "Port", request.Port)
		request.Username = promptString(reader, "Username", "")
		request.Password = promptString(reader, "Password", "")
		request.Database = promptString(reader, databasePromptLabel(request.Backend), "")
		if request.Backend == "oracle" && request.ExportMode == "schema" {
			request.Schema = promptString(reader, "Schema", "")
			request.Table = ""
		} else {
			request.Schema = promptString(reader, "Schema（可选）", "")
			request.Table = promptString(reader, "Table", "")
		}
	}

	request.OutputPath = promptOutputPath(reader, request)
	request.ConflictPolicy = promptSelect(reader, "文件冲突策略", []menuOption{
		{Value: "rename", Label: "自动重命名"},
		{Value: "overwrite", Label: "覆盖已有文件"},
	}, "rename")
	request.BatchSize = promptInt(reader, "Batch Size", request.BatchSize)
	request.Compression = promptSelect(reader, "Compression", compressionMenu(config.Compressions), request.Compression)

	fmt.Println()
	action := promptSelect(reader, "执行动作", []menuOption{
		{Value: "test", Label: "仅测试连接"},
		{Value: "export", Label: "直接导出"},
		{Value: "test_export", Label: "先测试连接再导出"},
	}, "test_export")

	for {
		if err := runCLIAction(reader, app, action, request); err != nil {
			if errors.Is(err, errExportCancelled) {
				return nil
			}
			return err
		}

		if action == "test" || isOracleSchemaExport(request) {
			return nil
		}

		nextTable := promptNextTable(reader)
		if nextTable == "" {
			return nil
		}

		request.Table = nextTable
		request.OutputPath = promptOutputPath(reader, request)
	}
}

type menuOption struct {
	Value string
	Label string
}

func promptSelectBackend(reader *bufio.Reader, options []appcore.Option, defaultValue string) string {
	menu := make([]menuOption, 0, len(options))
	for _, option := range options {
		menu = append(menu, menuOption{Value: option.Value, Label: option.Label})
	}
	return promptSelect(reader, "数据源类型", menu, defaultValue)
}

func compressionMenu(values []string) []menuOption {
	menu := make([]menuOption, 0, len(values))
	for _, value := range values {
		menu = append(menu, menuOption{Value: value, Label: strings.ToUpper(value)})
	}
	return menu
}

func promptSelect(reader *bufio.Reader, title string, options []menuOption, defaultValue string) string {
	fmt.Printf("%s:\n", title)
	defaultIndex := 1
	for idx, option := range options {
		if option.Value == defaultValue {
			defaultIndex = idx + 1
		}
		fmt.Printf("  %d) %s\n", idx+1, option.Label)
	}

	for {
		input := promptString(reader, fmt.Sprintf("请输入序号 [%d]", defaultIndex), "")
		if strings.TrimSpace(input) == "" {
			return options[defaultIndex-1].Value
		}

		index, err := strconv.Atoi(strings.TrimSpace(input))
		if err == nil && index >= 1 && index <= len(options) {
			return options[index-1].Value
		}
		fmt.Println("请输入有效序号。")
	}
}

func promptOutputPath(reader *bufio.Reader, request appcore.ExportRequest) string {
	suggested := strings.TrimSpace(request.OutputPath)
	if isOracleSchemaExport(request) {
		if suggested == "" {
			suggested = "schema-export"
		}
		schemaName := strings.TrimSpace(request.Schema)
		if schemaName != "" {
			suggested = filepath.Join(filepath.Dir(suggested), schemaName)
		}
		return promptString(reader, "输出目录路径", suggested)
	}

	if strings.TrimSpace(request.Table) != "" {
		fileName := strings.TrimSpace(request.Table) + ".parquet"
		suggested = filepath.Join(filepath.Dir(suggested), fileName)
	}
	return promptString(reader, "输出文件路径", suggested)
}

func promptNextTable(reader *bufio.Reader) string {
	fmt.Print("下一张表名（留空退出）: ")

	text, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}

	return strings.TrimSpace(text)
}

func runCLIAction(reader *bufio.Reader, app *appcore.App, action string, request appcore.ExportRequest) error {
	switch action {
	case "test":
		return app.RunTaskSync("test", request)
	case "export":
		confirmed, err := confirmBulkOracleSchemaExport(reader, app, request)
		if err != nil {
			return err
		}
		if !confirmed {
			return errExportCancelled
		}
		return app.RunTaskSync("export", request)
	default:
		if err := app.RunTaskSync("test", request); err != nil {
			return err
		}
		confirmed, err := confirmBulkOracleSchemaExport(reader, app, request)
		if err != nil {
			return err
		}
		if !confirmed {
			return errExportCancelled
		}
		return app.RunTaskSync("export", request)
	}
}

func confirmBulkOracleSchemaExport(
	reader *bufio.Reader,
	app *appcore.App,
	request appcore.ExportRequest,
) (bool, error) {
	if !isOracleSchemaExport(request) {
		return true, nil
	}

	preview, err := app.PreviewExport(request)
	if err != nil {
		return false, err
	}
	if preview.TableCount <= 10 {
		return true, nil
	}

	fmt.Fprintf(
		os.Stdout,
		"Schema %s 下共有 %d 张表，将全部导出。是否继续？[y/N]: ",
		strings.TrimSpace(preview.Schema),
		preview.TableCount,
	)
	text, err := reader.ReadString('\n')
	if err != nil {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func promptString(reader *bufio.Reader, label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}

	text, err := reader.ReadString('\n')
	if err != nil {
		return strings.TrimSpace(defaultValue)
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return strings.TrimSpace(defaultValue)
	}
	return text
}

func promptInt(reader *bufio.Reader, label string, defaultValue int) int {
	for {
		value := promptString(reader, label, strconv.Itoa(defaultValue))
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && parsed > 0 {
			return parsed
		}
		fmt.Println("请输入大于 0 的整数。")
	}
}

func databasePromptLabel(backend string) string {
	switch backend {
	case "oracle":
		return "Database / Service Name"
	default:
		return "Database"
	}
}

func isOracleSchemaExport(request appcore.ExportRequest) bool {
	return strings.EqualFold(strings.TrimSpace(request.Backend), "oracle") &&
		strings.EqualFold(strings.TrimSpace(request.ExportMode), "schema")
}

func printCLIEvent(event appcore.TaskEvent) {
	switch event.Type {
	case "running":
		if event.Message == "true" {
			fmt.Printf("[%s] 开始执行\n", event.Kind)
		} else {
			fmt.Printf("[%s] 执行结束\n", event.Kind)
		}
	case "log":
		fmt.Printf("[%s] %s\n", event.Kind, event.Message)
	case "progress":
		if event.Kind == "export" {
			fmt.Printf("[%s] 进度：已写入 %d 行\n", event.Kind, event.RowsWritten)
		}
	case "error":
		fmt.Printf("[%s] 错误: %s\n", event.Kind, event.Error)
	case "success":
		if event.Kind == "export" {
			fmt.Printf("[%s] 成功，输出文件: %s，写入行数: %d\n", event.Kind, event.OutputPath, event.RowsWritten)
			return
		}
		fmt.Printf("[%s] 成功\n", event.Kind)
	default:
		if event.Message != "" {
			fmt.Printf("[%s] %s\n", event.Kind, event.Message)
		}
	}
}
