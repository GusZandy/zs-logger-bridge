package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
		// Closing the window quits the app normally (default Wails
		// behavior -- no OnBeforeClose override). OnShutdown below still
		// stops the bridge's listeners cleanly before exit.
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("zs-logger-bridge error:", err.Error())
	}
}
