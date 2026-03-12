package main

import (
	"context"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) Startup(ctx context.Context) {
	a.core.SetEmitter(func(event TaskEvent) {
		wruntime.EventsEmit(ctx, "task:event", event)
	})
}
