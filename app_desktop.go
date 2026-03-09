//go:build desktop

package main

import (
	"context"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) startup(ctx context.Context) {
	a.SetEmitter(func(event TaskEvent) {
		wruntime.EventsEmit(ctx, "task:event", event)
	})
}
