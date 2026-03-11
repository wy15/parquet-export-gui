//go:build !desktop

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	app := NewApp()
	app.SetEmitter(printCLIEvent)

	if err := runInteractiveCLI(app); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func runInteractiveCLI(app *App) error {
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
		request.MaxComputeEndpoint = promptString(reader, "MaxCompute Endpoint", "https://service.cn-hangzhou.maxcompute.aliyun.com/api")
		request.MaxComputeProject = promptString(reader, "Project", "")
		request.MaxComputeAccessID = promptString(reader, "Access ID", "")
		request.MaxComputeAccessKey = promptString(reader, "Access Key", "")
		request.Schema = promptString(reader, "Schema（可选）", "")
		request.Table = promptString(reader, "Table", "")
		request.PartitionSpec = promptString(reader, "Partition Spec（可选）", "")
	} else {
		request.Host = promptString(reader, "Host", "127.0.0.1")
		request.Port = promptInt(reader, "Port", request.Port)
		request.Username = promptString(reader, "Username", "")
		request.Password = promptString(reader, "Password", "")
		request.Database = promptString(reader, databasePromptLabel(request.Backend), "")
		request.Schema = promptString(reader, "Schema（可选）", "")
		request.Table = promptString(reader, "Table", "")
	}

	request.OutputPath = promptOutputPath(reader, request.OutputPath, request.Table)
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

	switch action {
	case "test":
		return app.RunTaskSync("test", request)
	case "export":
		return app.RunTaskSync("export", request)
	default:
		if err := app.RunTaskSync("test", request); err != nil {
			return err
		}
		return app.RunTaskSync("export", request)
	}
}

type menuOption struct {
	Value string
	Label string
}

func promptSelectBackend(reader *bufio.Reader, options []Option, defaultValue string) string {
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

func promptOutputPath(reader *bufio.Reader, defaultPath, table string) string {
	suggested := defaultPath
	if strings.TrimSpace(table) != "" {
		fileName := strings.TrimSpace(table) + ".parquet"
		suggested = filepath.Join(filepath.Dir(defaultPath), fileName)
	}
	return promptString(reader, "输出文件路径", suggested)
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

func printCLIEvent(event TaskEvent) {
	switch event.Type {
	case "running":
		if event.Message == "true" {
			fmt.Printf("[%s] 开始执行\n", event.Kind)
		} else {
			fmt.Printf("[%s] 执行结束\n", event.Kind)
		}
	case "log":
		fmt.Printf("[%s] %s\n", event.Kind, event.Message)
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
