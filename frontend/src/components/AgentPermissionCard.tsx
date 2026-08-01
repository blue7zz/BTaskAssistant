import { CheckCircle2, Clock3, LoaderCircle, ShieldAlert, XCircle } from "lucide-react";
import type {
  AgentPermissionRequest,
  AgentPermissionScope,
} from "../domain/agent";

interface AgentPermissionCardProps {
  request: AgentPermissionRequest;
  submitting: boolean;
  onResolve: (
    decision: "allow" | "deny",
    scope: AgentPermissionScope | "",
  ) => void;
}

const SCOPE_LABELS: Record<AgentPermissionScope, string> = {
  once: "仅本次允许",
  session: "本会话允许",
  task: "当前任务允许",
  permanent: "永久允许",
};

const STATE_LABELS: Record<AgentPermissionRequest["state"], string> = {
  pending: "等待审批",
  allowed: "已允许",
  denied: "已拒绝",
  expired: "已超时",
  cancelled: "已取消",
};

const RISK_LABELS: Record<AgentPermissionRequest["riskLevel"], string> = {
  low: "低风险",
  medium: "中风险",
  high: "高风险",
  critical: "关键风险",
};

function allowedScopes(request: AgentPermissionRequest): AgentPermissionScope[] {
  const scopes = request.riskLevel === "critical"
    ? request.allowedScopes.filter((scope) => scope === "once")
    : request.allowedScopes;
  return Array.from(new Set(scopes));
}

export function AgentPermissionCard({
  request,
  submitting,
  onResolve,
}: AgentPermissionCardProps) {
  const pending = request.state === "pending";
  const Icon = pending
    ? ShieldAlert
    : request.state === "allowed"
      ? CheckCircle2
      : request.state === "denied"
        ? XCircle
        : Clock3;

  return (
    <article
      className={`agent-governance-card agent-permission-card risk-${request.riskLevel}`}
      aria-label={`权限审批 ${request.toolName}`}
    >
      <header>
        <span className="agent-governance-icon">
          <Icon size={14} />
        </span>
        <div>
          <strong>{request.toolName}</strong>
          <small>{request.capability}</small>
        </div>
        <span className={`agent-risk-badge risk-${request.riskLevel}`}>
          {RISK_LABELS[request.riskLevel]}
        </span>
        <span className={`agent-state-badge state-${request.state}`}>
          {STATE_LABELS[request.state]}
        </span>
      </header>

      <dl className="agent-governance-details">
        <div>
          <dt>目标</dt>
          <dd>{request.normalizedTarget || request.target || "未声明目标"}</dd>
        </div>
        <div>
          <dt>作用域</dt>
          <dd>
            任务 {request.taskId} · 会话 {request.sessionId} · 运行 {request.runId}
          </dd>
        </div>
        {request.reason && (
          <div>
            <dt>说明</dt>
            <dd>{request.reason}</dd>
          </div>
        )}
        {pending && request.expiresAt && (
          <div>
            <dt>时限</dt>
            <dd>{request.expiresAt}</dd>
          </div>
        )}
      </dl>

      {pending ? (
        <div className="agent-permission-actions" aria-label="权限审批操作">
          {allowedScopes(request).map((scope) => (
            <button
              key={scope}
              type="button"
              className="button secondary compact"
              disabled={submitting}
              onClick={() => onResolve("allow", scope)}
            >
              {submitting ? <LoaderCircle className="spin" size={11} /> : null}
              {SCOPE_LABELS[scope]}
            </button>
          ))}
          <button
            type="button"
            className="button compact agent-deny-button"
            disabled={submitting}
            onClick={() => onResolve("deny", "")}
          >
            拒绝
          </button>
        </div>
      ) : (
        <p className="agent-governance-result">
          {request.decisionScope
            ? `${STATE_LABELS[request.state]} · ${SCOPE_LABELS[request.decisionScope]}`
            : STATE_LABELS[request.state]}
        </p>
      )}
    </article>
  );
}
