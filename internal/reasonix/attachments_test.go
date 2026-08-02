package reasonix

import (
	"context"
	"strings"
	"testing"
	"time"

	"reasonix/bridge"
)

// TestAttachmentSaveAndRead 验证附件保存/读取：data URL → 任务附件目录 →
// 相对路径 → data URL 读回；路径归属校验拒绝越界。
func TestAttachmentSaveAndRead(t *testing.T) {
	dataRoot := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if _, err := manager.Ensure(ctx, "task_att", t.TempDir()); err != nil {
		t.Fatalf("Ensure 失败: %v", err)
	}
	// 1x1 红色 PNG
	rel, err := manager.SavePastedImage("task_att", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("SavePastedImage 失败: %v", err)
	}
	if !strings.HasPrefix(rel, "attachments/") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("附件相对路径异常: %s", rel)
	}
	// 读回
	dataURL, err := manager.AttachmentDataURL("task_att", rel)
	if err != nil {
		t.Fatalf("AttachmentDataURL 失败: %v", err)
	}
	if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("data URL 类型异常: %s", dataURL[:40])
	}
	// 越界路径拒绝
	if _, err := manager.AttachmentDataURL("task_att", "../../etc/passwd"); err == nil {
		t.Fatal("越界附件应拒绝")
	}
	if _, err := manager.AttachmentDataURL("task_att", "/etc/hosts"); err == nil {
		t.Fatal("绝对路径附件应拒绝")
	}
	manager.Shutdown()
}
