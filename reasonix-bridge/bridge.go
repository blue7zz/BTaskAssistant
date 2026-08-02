// Package bridge 是 BTaskAssistant 与 Reasonix 内核之间的适配层。
// module 名以 reasonix/ 开头以满足 Go internal 规则（reasonix/internal/* 仅允许
// reasonix/ 前缀的包导入）。所有任务级 Reasonix 会话能力从这里导出。
package bridge

import rxconfig "reasonix/internal/config"

// KernelInfo 验证内核可达性。
func KernelInfo() (string, error) {
	home := rxconfig.ReasonixHomeDir()
	if home == "" {
		return home, nil
	}
	return home, nil
}
