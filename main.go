package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "zs-logger Bridge",
		Width:     520,
		Height:    680,
		MinWidth:  420,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 20, G: 24, B: 31, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// Closing the window keeps the bridge running in the background
		// (listeners stay up) instead of quitting -- use the Quit button in
		// the UI, or the app/dock menu, to actually exit. This is a
		// lightweight stand-in for a true system tray icon; see README for
		// notes on adding one.
		OnBeforeClose: func(ctx context.Context) bool {
			runtime.WindowHide(ctx)
			return true
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("zs-logger-bridge error:", err.Error())
	}
}
