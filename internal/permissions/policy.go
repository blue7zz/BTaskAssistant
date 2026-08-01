package permissions

import (
	"path"
	"strings"
	"time"
)

func Evaluate(request Request, grants []Grant, now time.Time) Decision {
	classification := request.Classification
	decision := Decision{RiskLevel: classification.RiskLevel}
	if !request.GateValid {
		return hardDeny(classification.RiskLevel, "PI gate 身份或版本校验失败")
	}
	if strings.TrimSpace(request.TaskID) == "" || strings.TrimSpace(request.SessionID) == "" ||
		strings.TrimSpace(request.RunID) == "" || strings.TrimSpace(request.ToolCallID) == "" ||
		strings.TrimSpace(request.ToolName) == "" || strings.TrimSpace(classification.Capability) == "" ||
		strings.TrimSpace(classification.NormalizedTarget) == "" || strings.TrimSpace(classification.ArgsDigest) == "" {
		return hardDeny(classification.RiskLevel, "权限请求身份、目标或参数摘要不完整")
	}
	if classification.HardDenyReason != "" {
		return hardDeny(classification.RiskLevel, classification.HardDenyReason)
	}
	if strings.HasPrefix(classification.Capability, "shell.") {
		if request.Mode != "agent" {
			return hardDeny(classification.RiskLevel, "Ask/Plan 模式禁止申请 Shell")
		}
		if request.TaskStatus != "development" {
			return hardDeny(classification.RiskLevel, "Shell 仅可在 development 状态申请")
		}
	}
	if strings.HasPrefix(classification.Capability, "task.worktree.") && request.TaskStatus != "development" {
		return hardDeny(classification.RiskLevel, "worktree 写入仅可在 development 状态申请")
	}

	matching := make([]Grant, 0, len(grants))
	for _, grant := range grants {
		if grantMatches(grant, request, now) {
			matching = append(matching, grant)
		}
	}
	for _, grant := range matching {
		if grant.Decision == OutcomeDeny {
			decision.Outcome = OutcomeDeny
			decision.Reason = "匹配到显式拒绝授权"
			decision.MatchedGrantID = grant.ID
			return decision
		}
	}
	for _, grant := range matching {
		if grant.Scope == ScopeOnce && grant.Decision == OutcomeAllow {
			decision.Outcome = OutcomeAllow
			decision.Reason = "匹配到当前请求的一次性授权"
			decision.MatchedGrantID = grant.ID
			decision.ConsumeGrantID = grant.ID
			return decision
		}
	}
	if classification.RiskLevel == RiskCritical || classification.RiskLevel == RiskHigh {
		decision.Outcome = OutcomeAsk
		decision.Reason = "高风险操作必须逐次确认"
		decision.AllowedScopes = []Scope{ScopeOnce}
		return decision
	}
	for _, grant := range matching {
		if grant.Decision == OutcomeAllow {
			decision.Outcome = OutcomeAllow
			decision.Reason = "匹配到有效授权"
			decision.MatchedGrantID = grant.ID
			return decision
		}
	}

	switch classification.Capability {
	case "task.resource.list", "task.resource.read":
		decision.Outcome = OutcomeAllow
		decision.Reason = "当前任务只读资源由模式默认允许"
	case "task.artifact.write":
		if request.Mode == "plan" || request.Mode == "agent" {
			decision.Outcome = OutcomeAllow
			decision.Reason = "Plan/Agent 模式允许受控 artifact 写入"
		} else {
			decision.Outcome = OutcomeDeny
			decision.Reason = "Ask 模式禁止写入 artifact"
		}
	case "task.git.read":
		decision.Outcome = OutcomeAllow
		decision.Reason = "当前任务 worktree 的 Git 只读操作默认允许"
	default:
		decision.Outcome = OutcomeAsk
		decision.Reason = "没有匹配授权，需要用户确认"
		decision.AllowedScopes = scopesForRisk(classification.RiskLevel)
	}
	return decision
}

func scopesForRisk(risk RiskLevel) []Scope {
	if risk == RiskHigh || risk == RiskCritical {
		return []Scope{ScopeOnce}
	}
	return []Scope{ScopeOnce, ScopeSession, ScopeTask, ScopePermanent}
}

func AllowedScopes(risk RiskLevel) []Scope {
	return append([]Scope(nil), scopesForRisk(risk)...)
}

func grantMatches(grant Grant, request Request, now time.Time) bool {
	if grant.RevokedAt != nil || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
		return false
	}
	if grant.Capability != request.Classification.Capability ||
		riskRank(grant.RiskCeiling) < riskRank(request.Classification.RiskLevel) ||
		!targetMatches(grant.TargetPattern, request.Classification.NormalizedTarget) {
		return false
	}
	switch grant.Scope {
	case ScopeOnce:
		return grant.TaskID == request.TaskID && grant.SessionID == request.SessionID &&
			grant.RequestID == request.RequestID && grant.ConsumedAt == nil
	case ScopeSession:
		return grant.TaskID == request.TaskID && grant.SessionID == request.SessionID
	case ScopeTask:
		return grant.TaskID == request.TaskID && grant.SessionID == ""
	case ScopePermanent:
		return grant.TaskID == "" && grant.SessionID == "" &&
			request.Classification.RiskLevel != RiskHigh && request.Classification.RiskLevel != RiskCritical
	default:
		return false
	}
}

func targetMatches(pattern string, target string) bool {
	if pattern == target {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(target, strings.TrimSuffix(pattern, "*"))
	}
	matched, err := path.Match(pattern, target)
	return err == nil && matched
}

func hardDeny(risk RiskLevel, reason string) Decision {
	if risk == "" {
		risk = RiskCritical
	}
	return Decision{Outcome: OutcomeDeny, RiskLevel: risk, Reason: reason, HardDeny: true}
}

func riskRank(risk RiskLevel) int {
	switch risk {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 0
	}
}
