import { ChevronDown, ChevronRight, LoaderCircle, Wrench } from "lucide-react";
import { useState } from "react";
import type { AgentToolCall, AgentToolOutput } from "../domain/agent";

interface AgentToolCardProps {
  tool: AgentToolCall;
  loadOutput?: () => Promise<AgentToolOutput>;
}

const STATE_LABELS: Record<AgentToolCall["state"], string> = {
  received: "已接收",
  waiting_permission: "等待权限",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  denied: "已拒绝",
  cancelled: "已取消",
};

const RISK_LABELS: Record<AgentToolCall["riskLevel"], string> = {
  low: "低风险",
  medium: "中风险",
  high: "高风险",
  critical: "关键风险",
};

function durationText(tool: AgentToolCall): string {
  if (!tool.startedAt || !tool.finishedAt) return "";
  const startedAt = new Date(tool.startedAt).valueOf();
  const finishedAt = new Date(tool.finishedAt).valueOf();
  if (!Number.isFinite(startedAt) || !Number.isFinite(finishedAt) || finishedAt < startedAt) {
    return "";
  }
  const milliseconds = finishedAt - startedAt;
  if (milliseconds < 1000) return `${milliseconds} ms`;
  return `${(milliseconds / 1000).toFixed(1)} 秒`;
}

export function AgentToolCard({ tool, loadOutput }: AgentToolCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [output, setOutput] = useState<AgentToolOutput>();
  const [loadingOutput, setLoadingOutput] = useState(false);
  const [outputError, setOutputError] = useState("");

  const toggleDetails = async () => {
    const nextExpanded = !expanded;
    setExpanded(nextExpanded);
    if (
      !nextExpanded ||
      !tool.outputRef ||
      !loadOutput ||
      output ||
      loadingOutput ||
      outputError
    ) {
      return;
    }
    setLoadingOutput(true);
    try {
      setOutput(await loadOutput());
    } catch (reason) {
      setOutputError(reason instanceof Error ? reason.message : "读取工具输出失败");
    } finally {
      setLoadingOutput(false);
    }
  };

  return (
    <article
      className={`agent-governance-card agent-tool-card risk-${tool.riskLevel}`}
      aria-label={`工具调用 ${tool.toolName}`}
    >
      <button
        type="button"
        className="agent-tool-summary"
        aria-expanded={expanded}
        onClick={() => void toggleDetails()}
      >
        <span className="agent-governance-icon">
          <Wrench size={13} />
        </span>
        <span>
          <strong>{tool.toolName}</strong>
          <small>{tool.capability} · {tool.target || "未声明目标"}</small>
        </span>
        <span className={`agent-risk-badge risk-${tool.riskLevel}`}>
          {RISK_LABELS[tool.riskLevel]}
        </span>
        <span className={`agent-state-badge state-${tool.state}`}>
          {STATE_LABELS[tool.state]}
        </span>
        {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
      </button>

      {expanded && (
        <div className="agent-tool-details">
          <dl className="agent-governance-details">
            <div>
              <dt>工具调用</dt>
              <dd>{tool.externalToolCallId}</dd>
            </div>
            <div>
              <dt>运行</dt>
              <dd>{tool.runId}</dd>
            </div>
            {tool.startedAt && (
              <div>
                <dt>开始</dt>
                <dd>{tool.startedAt}</dd>
              </div>
            )}
            {durationText(tool) && (
              <div>
                <dt>耗时</dt>
                <dd>{durationText(tool)}</dd>
              </div>
            )}
          </dl>
          {tool.argsJson && (
            <section>
              <strong>参数</strong>
              <pre>{tool.argsJson}</pre>
            </section>
          )}
          {tool.argsRef && !tool.argsJson && <p>参数引用：{tool.argsRef}</p>}
          {tool.outputSummary && (
            <section>
              <strong>{tool.isError ? "错误" : "输出摘要"}</strong>
              <pre>{tool.outputSummary}</pre>
            </section>
          )}
          {loadingOutput && (
            <p className="agent-output-loading">
              <LoaderCircle className="spin" size={12} />
              正在读取完整输出…
            </p>
          )}
          {outputError && <p className="agent-output-error">{outputError}</p>}
          {output && (
            <section>
              <strong>
                完整输出{output.truncated ? "（已截断）" : ""}
              </strong>
              <pre>{output.content}</pre>
            </section>
          )}
          {tool.outputRef && !loadOutput && (
            <p>完整输出需要在原生客户端中读取。</p>
          )}
        </div>
      )}
    </article>
  );
}
