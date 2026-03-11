package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	azip "github.com/alexmullins/zip"
)

func (a *App) validateZipRequest(request ZipRequest) ([]GeneratedFile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(request.Files) == 0 {
		return nil, fmt.Errorf("请选择至少一个 Parquet 文件")
	}

	available := make(map[string]GeneratedFile, len(a.generatedFiles))
	for _, file := range a.generatedFiles {
		available[file.Path] = file
	}

	selected := make([]GeneratedFile, 0, len(request.Files))
	seen := make(map[string]struct{}, len(request.Files))
	for _, rawPath := range request.Files {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}

		file, ok := available[path]
		if !ok {
			return nil, fmt.Errorf("文件不在当前会话列表中: %s", filepath.Base(path))
		}
		if strings.ToLower(filepath.Ext(file.Path)) != ".parquet" {
			return nil, fmt.Errorf("仅支持打包 parquet 文件: %s", file.Name)
		}

		seen[path] = struct{}{}
		selected = append(selected, file)
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("请选择至少一个 Parquet 文件")
	}
	return selected, nil
}

func buildZipOutputPath(firstFile GeneratedFile) string {
	dir := filepath.Dir(firstFile.Path)
	base := fmt.Sprintf("parquet-export-%s.zip", time.Now().Format("20060102-150405"))
	outputPath := filepath.Join(dir, base)

	index := 1
	for {
		if _, err := os.Stat(outputPath); os.IsNotExist(err) {
			return outputPath
		}
		outputPath = filepath.Join(dir, fmt.Sprintf("parquet-export-%s-%d.zip", time.Now().Format("20060102-150405"), index))
		index++
	}
}

func writeZipArchive(outputPath string, files []GeneratedFile, password string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := azip.NewWriter(out)

	for _, file := range files {
		source, err := os.Open(file.Path)
		if err != nil {
			return err
		}

		var entry io.Writer
		if strings.TrimSpace(password) != "" {
			entry, err = writer.Encrypt(file.Name, strings.TrimSpace(password))
		} else {
			entry, err = writer.Create(file.Name)
		}
		if err != nil {
			source.Close()
			return err
		}

		if _, err := io.Copy(entry, source); err != nil {
			source.Close()
			return err
		}
		if err := source.Close(); err != nil {
			return err
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}
	return nil
}
