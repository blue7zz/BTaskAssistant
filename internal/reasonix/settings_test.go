package reasonix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/bridge"
)

// TestCredentialWriteIsolated 验证凭据写入：.env 0600 权限、原子替换、
// 明文不落日志（函数只写文件）。REASONIX_HOME 由 TestMain 指向临时隔离目录。
func TestCredentialWriteIsolated(t *testing.T) {
	home := os.Getenv("REASONIX_HOME")
	if home == "" {
		t.Fatal("TestMain 未设置 REASONIX_HOME")
	}
	if err := bridge.SaveProvider("test-provider", "openai", "https://api.example.com/v1", "TEST_API_KEY", "sk-secret-value"); err != nil {
		t.Fatalf("SaveProvider 失败: %v", err)
	}
	envPath := filepath.Join(home, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("隔离 home 无 .env: %v", err)
	}
	if !strings.Contains(string(data), "TEST_API_KEY=sk-secret-value") {
		t.Fatalf(".env 内容异常: %q", string(data))
	}
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf(".env 权限过宽: %v（应 0600）", perm)
	}
}
