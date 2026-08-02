package reasonix

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"reasonix/bridge"
)

// procRSS 读取本进程 RSS（KB）。
func procRSS() uint64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return 0
	}
	value, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	return value
}

// TestSessionMemoryFootprint 测量单个 Reasonix 会话的内存占用（进程内存增量）。
func TestSessionMemoryFootprint(t *testing.T) {
	procRSS := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapInuse
	}

	baseline := procRSS()
	rssBaseline := procRSS()
	dataRoot := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 1 个会话
	if _, err := manager.Ensure(ctx, "task_mem1", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	after1 := procRSS()
	rssAfter1 := procRSS()
	t.Logf("基线 heap: %.1f MiB / RSS: %.1f MiB", float64(baseline)/1024/1024, float64(rssBaseline)/1024/1024)
	t.Logf("1 会话后 heap: %.1f MiB（增量 %.1f MiB）/ RSS: %.1f MiB（增量 %.1f MiB）",
		float64(after1)/1024/1024, float64(after1-baseline)/1024/1024,
		float64(rssAfter1)/1024/1024, float64(rssAfter1-rssBaseline)/1024/1024)

	// 3 个更多会话（共 4 个，不超过 idle 上限 MaxIdleRuntimes——避免被自动回收）
	for i := 2; i <= 4; i++ {
		if _, err := manager.Ensure(ctx, "task_mem"+string(rune('0'+i)), t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	after5 := procRSS()
	rssAfter5 := procRSS()
	t.Logf("4 会话后 heap: %.1f MiB（累计增量 %.1f MiB，平均每会话 %.1f MiB）/ RSS: %.1f MiB（累计增量 %.1f MiB，平均每会话 %.1f MiB）",
		float64(after5)/1024/1024,
		float64(after5-baseline)/1024/1024,
		float64(after5-after1)/3/1024/1024,
		float64(rssAfter5)/1024/1024,
		float64(rssAfter5-rssBaseline)/1024/1024,
		float64(rssAfter5-rssAfter1)/3/1024/1024)

	// 会话活跃后（提交一轮）的内存
	if err := manager.Submit("task_mem1", "内存测量"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	runtime.GC()
	afterTurn := procRSS()
	t.Logf("活跃回合后 heap: %.1f MiB（增量 %.1f MiB）", float64(afterTurn)/1024/1024, float64(afterTurn-after5)/1024/1024)

	manager.Shutdown()
}
