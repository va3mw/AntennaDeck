package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	cfg := app.config()
	w, h := cfg.windowSize()

	err := wails.Run(&options.App{
		Title:            "Beam Controller",
		Width:            w,
		Height:           h,
		MinWidth:         300,
		MinHeight:        200,
		AlwaysOnTop:      cfg.AlwaysOnTop,
		BackgroundColour: &options.RGBA{R: 20, G: 18, B: 16, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
