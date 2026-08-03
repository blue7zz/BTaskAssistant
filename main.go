package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if os.Getenv("BTA_RX_SELFCHECK") == "1" {
		os.Exit(runRXSelfCheck())
	}

	app := NewApp()

	frontendFS, err := fsSub(assets, "frontend/dist")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}

	// Reasonix 前端已完全嵌入（shadow DOM 同构建）——不再提供 /reasonix/ 路由。
	assetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.FileServer(http.FS(frontendFS)).ServeHTTP(w, r)
	})

	err = wails.Run(&options.App{
		Title:                    "BTaskAssistant",
		Width:                    1440,
		Height:                   920,
		MinWidth:                 1120,
		MinHeight:                720,
		AssetServer:              &assetserver.Options{Handler: assetHandler},
		BackgroundColour:         &options.RGBA{R: 245, G: 246, B: 249, A: 255},
		OnStartup:                app.startup,
		OnShutdown:               app.shutdown,
		Bind:                     []interface{}{app},
		EnableDefaultContextMenu: false,
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.DefaultAppearance,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func fsSub(embedded embed.FS, path string) (fs.FS, error) {
	return fs.Sub(embedded, path)
}
