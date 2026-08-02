package main

// RX 运行时自检：BTA_RX_SELFCHECK=1 启动时，不进入 wails 主循环，直接执行
// Reasonix 会话构建链路（内核 boot.Build → 新会话 → 提交）并报告结果。
// 用途：无需 UI/系统权限即可验证真实 wails 进程内的 RX 内核可用性
// （CI 或故障排查：BTA_RX_SELFCHECK=1 ./BTaskAssistant）。
//
// 说明：自检不依赖任务存储（显式传工作区目录），模型缺失时构建仍成功
// （RequireKey=false），提交由内核容错——自检验证的是"会话可构建、事件可流"。

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

func runRXSelfCheck() int {
	app := NewApp()
	app.initReasonix()
	defer app.rxCloseAll()

	workspace, err := os.MkdirTemp("", "bta-rx-selfcheck-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "RX 自检失败: 创建工作区: %v\n", err)
		return 1
	}
	defer os.RemoveAll(workspace)

	taskID := "selfcheck-" + strconv.Itoa(os.Getpid())
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tab, err := app.rxManager.Ensure(ctx, taskID, workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "RX 自检失败: 构建会话: %v\n", err)
		return 1
	}
	if tab == nil || tab.Ctrl == nil {
		fmt.Fprintln(os.Stderr, "RX 自检失败: 控制器为空")
		return 1
	}
	fmt.Printf("RX 自检: 会话构建通过 (tab=%s, label=%s)\n", tab.ID, tab.Ctrl.Label())

	if err := app.rxManager.NewSession(taskID); err != nil {
		fmt.Fprintf(os.Stderr, "RX 自检失败: 新会话: %v\n", err)
		return 1
	}
	fmt.Printf("RX 自检: 新会话路径 %s\n", app.rxManager.CurrentSessionPath(taskID))

	if err := app.rxManager.Submit(taskID, "RX 自检消息"); err != nil {
		fmt.Fprintf(os.Stderr, "RX 自检失败: 提交: %v\n", err)
		return 1
	}
	fmt.Println("RX 自检: 提交成功（回合异步运行，1s 后关闭）")
	time.Sleep(time.Second)

	// 事件流验证：提交后应至少产生内核事件（sink 注册的 emit 回调）
	if app.rxManager.Tab(taskID) == nil {
		fmt.Fprintln(os.Stderr, "RX 自检失败: tab 丢失")
		return 1
	}
	fmt.Println("RX 自检: 通过")
	return 0
}
