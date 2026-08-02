package reasonix

import (
	"os"
	"testing"
)

// TestMain 隔离测试环境：所有 reasonix 测试使用临时 REASONIX_HOME，
// 不读取本机真实配置、不消耗真实 API 额度（RequireKey=false 容错）。
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "btask-rx-test-home-*")
	if err != nil {
		os.Exit(1)
	}
	os.Setenv("REASONIX_HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Unsetenv("REASONIX_HOME")
	os.Exit(code)
}
