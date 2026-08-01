package permissions

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var (
	criticalCommand         = regexp.MustCompile(`(?i)(^|[;&|\s])(?:git\s+(?:push|rebase|reset|filter-branch|filter-repo)|gh\s+(?:pr\s+create|release)|(?:npm|pnpm|yarn|cargo)\s+publish)(?:\s|$)`)
	destructiveCommand      = regexp.MustCompile(`(?i)(^|[;&|\s])(?:rm\s+(?:-[a-z]*r[a-z]*f|-rf|-fr)|git\s+clean\s+[^;&|]*(?:-[a-z]*f)|gh\s+repo\s+delete)(?:\s|$)`)
	remoteDeleteCommand     = regexp.MustCompile(`(?i)(^|[;&|\s])curl\b[^;&|]*(?:-X\s*|--request(?:=|\s+))DELETE(?:\s|$)`)
	credentialCommand       = regexp.MustCompile(`(?i)(?:\.ssh|id_rsa|id_ed25519|security\s+find-(?:generic|internet)-password|(?:printenv|env)\s+.*(?:token|secret|password)|(?:authorization|token|pat|password|secret|api[_-]?key|cookie|credential)\s*[:=])`)
	dependencyCommand       = regexp.MustCompile(`(?i)(^|[;&|\s])(?:(?:npm|pnpm|yarn)\s+(?:install|add|update)|cargo\s+(?:add|install)|go\s+get)(?:\s|$)`)
	indirectCommand         = regexp.MustCompile("(?i)(?:[|<>`]|\\$\\(|&&|\\|\\||(^|[;\\s])(?:eval|source|xargs|sudo|ssh|curl|wget)([;\\s]|$)|find\\s+.*-exec|(?:npm|pnpm|yarn)\\s+(?:run|exec)|(?:sh|bash|zsh)\\s+-c)")
	unsupportedGitShell     = regexp.MustCompile(`(?i)(^|[;&|\s])git\s+(?:add|commit|push|pull|fetch|merge|rebase|reset|clean|checkout|switch|branch|tag|remote|worktree|submodule|filter-branch|filter-repo)(?:\s|$)`)
	unsupportedPublishShell = regexp.MustCompile(`(?i)(^|[;&|\s])(?:gh\s+(?:pr|release|repo)|(?:npm|pnpm|yarn|cargo)\s+publish)(?:\s|$)`)
)

func ClassifyCommand(command string, cwdTarget string) Classification {
	command = strings.TrimSpace(command)
	if command == "" || cwdTarget == "" || strings.ContainsRune(command, '\x00') || strings.ContainsRune(cwdTarget, '\x00') {
		return deniedClassification("Shell command 或 cwd 无法规范化")
	}
	digest, _ := CanonicalArgsDigest([]byte(`{"command":` + quoteJSON(command) + `,"cwd":` + quoteJSON(cwdTarget) + `}`))
	classification := Classification{
		Capability: "shell.execute", Subject: "在当前任务 worktree 执行命令",
		Target: command, NormalizedTarget: cwdTarget + "\x00" + command,
		ArgsDigest: digest, RiskLevel: RiskMedium, Mutating: true,
	}
	if credentialCommand.MatchString(command) {
		classification.RiskLevel = RiskCritical
		classification.HardDenyReason = "命令可能读取凭据"
		classification.Target = "[REDACTED credential-bearing command]"
		classification.NormalizedTarget = cwdTarget + "\x00[REDACTED:" + digest + "]"
		return classification
	}
	if criticalCommand.MatchString(command) || destructiveCommand.MatchString(command) || remoteDeleteCommand.MatchString(command) {
		classification.RiskLevel = RiskCritical
		classification.Subject = "执行关键 Git、发布、远端或大量删除操作"
		return classification
	}
	if dependencyCommand.MatchString(command) {
		classification.RiskLevel = RiskHigh
		classification.Subject = "安装或更新项目依赖"
	}
	if indirectCommand.MatchString(command) || strings.Contains(command, "(") || strings.Contains(command, ")") || strings.Contains(command, "\n") {
		classification.RiskLevel = RiskHigh
		classification.Subject = "执行含重定向、管道或间接调用的命令"
	}
	return classification
}

func unsupportedShellAction(command string) bool {
	return unsupportedGitShell.MatchString(command) || unsupportedPublishShell.MatchString(command) || remoteDeleteCommand.MatchString(command)
}

func ClassifyGit(args []string, target string) (Classification, error) {
	if len(args) == 0 || strings.TrimSpace(target) == "" {
		return Classification{}, errors.New("Git action and target are required")
	}
	joined := "git " + strings.Join(args, " ")
	classification := ClassifyCommand(joined, target)
	action := strings.ToLower(args[0])
	switch action {
	case "status", "diff", "log", "show":
		classification.Capability = "task.git.read"
		classification.Subject = "读取当前任务 Git 状态"
		classification.RiskLevel = RiskLow
		classification.ReadOnly = true
		classification.Mutating = false
	case "add", "commit", "branch":
		classification.Capability = "task.git.write"
		classification.Subject = "修改当前任务本地 Git 状态"
		classification.RiskLevel = RiskHigh
	case "push":
		classification.Capability = "git.remote.push"
		classification.Subject = "推送当前任务分支到远端"
		classification.RiskLevel = RiskCritical
	case "rebase", "reset", "filter-branch", "filter-repo":
		classification.Capability = "git.history.rewrite"
		classification.Subject = "重写当前任务 Git 历史"
		classification.RiskLevel = RiskCritical
	case "clean":
		classification.Capability = "task.git.destructive"
		classification.Subject = "清理当前任务 Git 工作区"
		classification.RiskLevel = RiskCritical
	default:
		classification.Capability = "task.git.unknown"
		classification.Subject = "执行未分类 Git 操作"
		classification.RiskLevel = RiskHigh
	}
	return classification, nil
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
