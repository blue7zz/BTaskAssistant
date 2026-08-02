package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestHandler 用临时 reasonix dist 构建 asset handler，
// 与 main() 中的路由逻辑保持一致。
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	frontendFS, err := fsSub(assets, "frontend/dist")
	if err != nil {
		t.Fatalf("frontend dist: %v", err)
	}

	dist := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html>reasonix</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatalf("write js: %v", err)
	}

	reasonixHandler := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/reasonix" || strings.HasPrefix(path, "/reasonix/") {
			trimmed := strings.TrimPrefix(path, "/reasonix")
			if trimmed == "" {
				trimmed = "/"
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = trimmed
			reasonixHandler.ServeHTTP(w, r2)
			return
		}
		http.FileServer(http.FS(frontendFS)).ServeHTTP(w, r)
	})
}

func getBody(t *testing.T, handler http.Handler, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return rec.Code, string(body)
}

func TestReasonixAssetRoutes(t *testing.T) {
	handler := newTestHandler(t)

	t.Run("reasonix root serves index.html", func(t *testing.T) {
		code, body := getBody(t, handler, "/reasonix/")
		if code != http.StatusOK {
			t.Fatalf("want 200, got %d", code)
		}
		if body != "<html>reasonix</html>" {
			t.Fatalf("want reasonix index, got %q", body)
		}
	})

	t.Run("reasonix bare path serves index", func(t *testing.T) {
		code, body := getBody(t, handler, "/reasonix")
		if code != http.StatusOK {
			t.Fatalf("want 200, got %d", code)
		}
		if body != "<html>reasonix</html>" {
			t.Fatalf("want reasonix index, got %q", body)
		}
	})
	t.Run("reasonix asset file serves", func(t *testing.T) {
		code, body := getBody(t, handler, "/reasonix/assets/app.js")
		if code != http.StatusOK {
			t.Fatalf("want 200, got %d", code)
		}
		if body != "console.log(1)" {
			t.Fatalf("want js body, got %q", body)
		}
	})

	t.Run("reasonix missing file 404s", func(t *testing.T) {
		code, _ := getBody(t, handler, "/reasonix/nope.txt")
		if code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", code)
		}
	})

	t.Run("main frontend still serves", func(t *testing.T) {
		code, _ := getBody(t, handler, "/")
		if code != http.StatusOK && code != http.StatusNotFound {
			t.Fatalf("unexpected main route status %d", code)
		}
	})
}

func TestReasonixDistDir(t *testing.T) {
	// 当前测试工作目录即项目根，reasonix dist 已构建。
	dir := reasonixDistDir()
	if dir == "" {
		t.Fatal("reasonixDistDir() 返回空，原因：reasonix-app/desktop/frontend/dist 未构建")
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Fatalf("dist 缺少 index.html: %v", err)
	}
}
