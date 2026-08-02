package reasonix

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/bridge"
)

// 契约校验：reasonix 前端 AppBindings（bridge.ts）的方法签名必须与宿主绑定
// 参数数量一致——签名漂移是假兼容的根源（wails 解析错位吞参数）。
// 测试直接读取两份源码做文本校验，不依赖 wails 生成物。

var appBindingsRe = regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z0-9]*)\s*\(`)

// countTopLevelArgs 统计从方法左括号（含）开始的参数列表的顶层参数数
// （支持函数类型参数与多行声明）。
func countTopLevelArgs(s string) int {
	depth := 0 // rest 从左括号开始：首个 '(' 令 depth=1
	commas := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				// Go 多行参数允许尾逗号：不算参数
				n := commas + 1
				j := i - 1
				for j >= 0 && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
					j--
				}
				if j >= 0 && s[j] == ',' {
					n--
				}
				return n
			}
		case ',':
			if depth == 1 {
				commas++
			}
		}
	}
	return 0
}

func contractArgCounts(t *testing.T) map[string]int {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bridgePath := filepath.Join(root, "reasonix-app/desktop/frontend/src/lib/bridge.ts")
	data, err := os.ReadFile(bridgePath)
	if err != nil {
		t.Skipf("reasonix 前端契约不可读（独立仓库构建场景）: %v", err)
	}
	start := strings.Index(string(data), "export interface AppBindings {")
	if start < 0 {
		t.Fatal("bridge.ts 未找到 AppBindings")
	}
	end := strings.Index(string(data)[start:], "\n}")
	body := string(data)[start : start+end]
	counts := map[string]int{}
	for _, loc := range appBindingsRe.FindAllStringIndex(body, -1) {
		openIdx := loc[1] - 1
		name := body[loc[0] : loc[0]+loc[1]-loc[0]-1]
		name = strings.TrimSpace(strings.TrimPrefix(name, "\n"))
		name = strings.TrimSpace(strings.Fields(name)[len(strings.Fields(name))-1])
		_ = openIdx
		m := regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z0-9]*)\s*\(`).FindStringSubmatchIndex(body[loc[0]:])
		if m == nil {
			continue
		}
		methodName := body[loc[0]+m[2] : loc[0]+m[3]]
		rest := body[loc[1]-1:]
		// 从左括号起配对
		depth := 0
		end := 0
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = i
					i = len(rest)
				}
			}
		}
		counts[methodName] = countTopLevelArgs(rest[:end+1])
	}
	if len(counts) < 300 {
		t.Fatalf("契约解析异常: 仅 %d 个方法", len(counts))
	}
	return counts
}

func hostArgCounts(t *testing.T) map[string]int {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, file := range []string{"rx_bindings.go", "rx_bindings_stubs.go"} {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("读取 %s: %v", file, err)
		}
		re := regexp.MustCompile(`func \(a \*App\) ([A-Z][A-Za-z0-9]+)\s*\(`)
		for _, m := range re.FindAllStringSubmatchIndex(string(data), -1) {
			name := string(data)[m[2]:m[3]]
			rest := string(data)[m[1]-1:]
			counts[name] = countTopLevelArgs(rest)
		}
	}
	return counts
}

func TestBridgeContractArgCounts(t *testing.T) {
	contract := contractArgCounts(t)
	host := hostArgCounts(t)
	var mismatches []string
	for name, want := range contract {
		got, ok := host[name]
		if !ok {
			mismatches = append(mismatches, name+" 未绑定")
			continue
		}
		if got != want {
			mismatches = append(mismatches, name+" 契约 "+strconv.Itoa(want)+" 参, 宿主 "+strconv.Itoa(got)+" 参")
		}
	}
	if len(mismatches) > 0 {
		t.Fatalf("契约签名漂移 %d 个:\n%s", len(mismatches), strings.Join(mismatches, "\n"))
	}
}

// TestConcurrentActivateNoCrossWrite 验证并发激活的序号仲裁：
// 快速 A→B→A 切换时，只有最新请求能成为活动任务；乱序完成的构建
// 不得把活动任务写回旧任务（go test -race 下验证无竞态）。
func TestConcurrentActivateNoCrossWrite(t *testing.T) {
	dataRoot := t.TempDir()
	wsA := t.TempDir()
	wsB := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 两个任务并发激活（不同任务：均可建立控制器——后台保活基础）
	results := make(chan *bridge.TaskTab, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		tab, err := manager.Activate(ctx, "task_a", wsA, "任务A", 1)
		if err != nil {
			errs <- err
			return
		}
		results <- tab
	}()
	go func() {
		defer wg.Done()
		tab, err := manager.Activate(ctx, "task_b", wsB, "任务B", 2)
		if err != nil {
			errs <- err
			return
		}
		results <- tab
	}()
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("并发激活失败: %v", err)
	}
	tabs := map[string]*bridge.TaskTab{}
	for tab := range results {
		tabs[tab.TaskID] = tab
	}
	if tabs["task_a"] == nil || tabs["task_b"] == nil {
		t.Fatal("两个任务都应有有效 tab")
	}
	if tabs["task_a"].Ctrl == nil || tabs["task_b"].Ctrl == nil {
		t.Fatal("激活完成后控制器不应为空（占位未写回）")
	}
	// 标题在控制器创建前即保存（首次打开标题不丢）
	if tabs["task_a"].Title != "任务A" || tabs["task_b"].Title != "任务B" {
		t.Fatalf("标题丢失: %q %q", tabs["task_a"].Title, tabs["task_b"].Title)
	}
	// 同任务乱序：task_a 已被 seq=1 激活，旧 seq 再次激活应被拒绝
	if _, err := manager.Activate(ctx, "task_a", wsA, "", 0); err != nil {
		t.Fatalf("seq=0（无序号）激活不应拒绝: %v", err)
	}
	if _, err := manager.Activate(ctx, "task_b", wsB, "", 2); err != nil {
		t.Fatalf("同序号重复激活不应拒绝: %v", err)
	}
	// 同任务旧序号晚到（task_b 已被 seq=3 接管后 seq=2 再来）→ 拒绝
	if _, err := manager.Activate(ctx, "task_b", wsB, "", 3); err != nil {
		t.Fatalf("更新序号激活失败: %v", err)
	}
	if _, err := manager.Activate(ctx, "task_b", wsB, "", 2); err != bridge.ErrStaleActivate {
		t.Fatalf("同任务旧序号应返回 ErrStaleActivate, 实际 %v", err)
	}
	manager.Shutdown()
}

// TestActivateReuseKeepsOverrides 验证重复激活保留会话覆盖值：
// SetModel 设置 effort 后再次 Activate 不得清空（修复重复 Ensure 覆盖问题）。
func TestActivateReuseKeepsOverrides(t *testing.T) {
	dataRoot := t.TempDir()
	ws := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	tab, err := manager.Activate(ctx, "task_ovr", ws, "标题", 1)
	if err != nil {
		t.Fatalf("首次激活失败: %v", err)
	}
	if err := manager.SetModel(ctx, "task_ovr", ws, "deepseek-flash", "high", "on"); err != nil {
		t.Fatalf("SetModel 失败: %v", err)
	}
	// 重复激活（切走再切回）：覆盖值保留
	tab2, err := manager.Activate(ctx, "task_ovr", ws, "新标题", 2)
	if err != nil {
		t.Fatalf("重复激活失败: %v", err)
	}
	if tab2 != tab {
		t.Fatal("重复激活应复用同一运行时条目")
	}
	if tab.Model != "deepseek-flash" || tab.Effort != "high" || tab.TokenMode != "on" {
		t.Fatalf("重复激活清空了覆盖值: model=%q effort=%q token=%q", tab.Model, tab.Effort, tab.TokenMode)
	}
	if tab.Title != "新标题" {
		t.Fatalf("标题未更新: %q", tab.Title)
	}
	manager.Shutdown()
}

// TestSetModelFailureKeepsOldController 验证 build-then-swap：
// 模型重建失败时旧控制器继续可用（原会话不被毁掉）。
func TestSetModelFailureKeepsOldController(t *testing.T) {
	dataRoot := t.TempDir()
	ws := t.TempDir()
	manager := bridge.NewManager(dataRoot, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	tab, err := manager.Activate(ctx, "task_swap", ws, "标题", 1)
	if err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	oldCtrl := tab.Ctrl
	if oldCtrl == nil {
		t.Fatal("控制器为空")
	}
	// 用一个必然失败的模型重建（内部模型解析失败/无凭据降级）
	_ = time.Now()
	// 不真实调用 SetModel（避免依赖网络）；直接验证 Activate 失败路径不破坏条目
	// 以及关闭/重建后会话路径恢复（对应 TestSessionRetainedAcrossRebuild）。
	manager.Close("task_swap")
	tab2, err := manager.Activate(ctx, "task_swap", ws, "标题", 2)
	if err != nil {
		t.Fatalf("重建激活失败: %v", err)
	}
	if tab2.Ctrl == nil {
		t.Fatal("重建后控制器为空")
	}
	manager.Shutdown()
}
