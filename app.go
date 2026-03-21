package main

import "parquet-export-gui/internal/appcore"

type Option = appcore.Option
type ExportRequest = appcore.ExportRequest
type AppConfig = appcore.AppConfig
type ExportOutputCheck = appcore.ExportOutputCheck
type ExportPreview = appcore.ExportPreview
type GeneratedFile = appcore.GeneratedFile
type ZipRequest = appcore.ZipRequest
type ZipResult = appcore.ZipResult
type TaskEvent = appcore.TaskEvent

type App struct {
	core *appcore.App
}

func NewApp() *App {
	return &App{core: appcore.NewApp()}
}

func (a *App) GetConfig() AppConfig {
	return a.core.GetConfig()
}

func (a *App) StartTask(kind string, request ExportRequest) error {
	return a.core.StartTask(kind, request)
}

func (a *App) GetGeneratedFiles() []GeneratedFile {
	return a.core.GetGeneratedFiles()
}

func (a *App) CreateZipArchive(request ZipRequest) (ZipResult, error) {
	return a.core.CreateZipArchive(request)
}

func (a *App) CheckExportOutput(path string) (ExportOutputCheck, error) {
	return a.core.CheckExportOutput(path)
}

func (a *App) PreviewExport(request ExportRequest) (ExportPreview, error) {
	return a.core.PreviewExport(request)
}
