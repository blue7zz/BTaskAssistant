package reasonix

import (
	"context"
	"os"
	"testing"
	"time"

	"reasonix/bridge"
)

// TestEnsureWithoutModelConfig 验证无任何 reasonix 配置（无模型/provider）时
// 会话构建的行为——用户机器未配置时应降级而非硬失败。
func TestEnsureWithoutModelConfig(t *testing.T) {
	emptyHome := t.TempDir()
	t.Setenv("REASONIX_HOME", emptyHome)
	t.Setenv("HOME", emptyHome)

	dataRoot := t.TempDir()
	workspace := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	tab, err := manager.Ensure(ctx, "task_nocfg", workspace)
	if err != nil {
		t.Fatalf("无配置环境 Ensure 失败: %v", err)
	}
	if tab == nil || tab.Ctrl == nil {
		t.Fatal("无配置环境控制器为空")
	}
	// 界面可达（RequireKey=false 的契约）：提交不应因缺模型而 panic
	_ = manager.Submit("task_nocfg", "你好")
	time.Sleep(500 * time.Millisecond)
	manager.Close("task_nocfg")
	_ = os.Getenv("REASONIX_HOME")
}
