package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// reasonixDistDir 定位 reasonix-app/desktop/frontend/dist（reasonix-app 是独立
// Go module，无法 go:embed，改为运行时从磁盘提供）。开发时 cwd 是项目根；
// 打包后从可执行文件路径向上查找项目目录。
func reasonixDistDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for range 6 {
			dir = filepath.Dir(dir)
			candidate := filepath.Join(dir, "reasonix-app", "desktop", "frontend", "dist")
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				return candidate
			}
		}
	}
	if info, statErr := os.Stat("reasonix-app/desktop/frontend/dist"); statErr == nil && info.IsDir() {
		return "reasonix-app/desktop/frontend/dist"
	}
	return ""
}

func main() {
	if os.Getenv("BTA_RX_SELFCHECK") == "1" {
		os.Exit(runRXSelfCheck())
	}

	app := NewApp()

	frontendFS, err := fsSub(assets, "frontend/dist")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	reasonixDir := reasonixDistDir()
	reasonixHandler := http.NotFoundHandler()
	if reasonixDir != "" {
		log.Printf("Reasonix 前端 dist: %s", reasonixDir)
		reasonixHandler = http.FileServer(http.Dir(reasonixDir))
	}

	// Reasonix 前端在同源 /reasonix/ 路径下提供，BTask 任务详情的 RX 标签页用
	// iframe 直接加载（reasonix 前端在无 wails 桥时自动进入 mock 模式，
	// 完整 UI 仍可交互预览）。
	assetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/reasonix" || strings.HasPrefix(path, "/reasonix/") {
			trimmed := strings.TrimPrefix(path, "/reasonix")
			if trimmed == "" {
				trimmed = "/"
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = trimmed
			reasonixHandler.ServeHTTP(w, r2)
			// 诊断：reasonix 子资源 404 时记录（iframe 白屏排查入口）
			if reasonixDir != "" && r2.URL.Path != "/" && !strings.HasPrefix(r2.URL.Path, "/assets/") {
				if _, err := os.Stat(filepath.Join(reasonixDir, filepath.FromSlash(strings.TrimPrefix(r2.URL.Path, "/")))); err != nil {
					log.Printf("reasonix 资源缺失: %s", r2.URL.Path)
				}
			}
			return
		}
		http.FileServer(http.FS(frontendFS)).ServeHTTP(w, r)
	})

	err = wails.Run(&options.App{
		Title:                    "BTaskAssistant",
		Width:                    1440,
		Height:                   920,
		MinWidth:                 1120,
		MinHeight:                720,
		AssetServer:              &assetserver.Options{Handler: assetHandler},
		BackgroundColour:         &options.RGBA{R: 246, G: 247, B: 251, A: 1},
		OnStartup:                app.startup,
		OnShutdown:               app.shutdown,
		Bind:                     []interface{}{app},
		EnableDefaultContextMenu: false,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func fsSub(embedded embed.FS, path string) (fs.FS, error) {
	return fs.Sub(embedded, path)
}
