import { Activity, LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { AgentRun } from "../domain/agent";
import type { AgentClient } from "../lib/agentBridge";

interface AgentRunsPanelProps {
  taskId: string;
  sessionId: string;
  client: AgentClient;
  refreshVersion: number;
}

function duration(run: AgentRun): string {
  if (!run.finishedAt) return "运行中";
  const value = new Date(run.finishedAt).valueOf() - new Date(run.startedAt).valueOf();
  if (!Number.isFinite(value) || value < 0) return "";
  if (value < 1000) return `${value} ms`;
  return `${(value / 1000).toFixed(1)} 秒`;
}

export function AgentRunsPanel({
  taskId,
  sessionId,
  client,
  refreshVersion,
}: AgentRunsPanelProps) {
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const epochRef = useRef(0);

  useEffect(() => {
    const epoch = ++epochRef.current;
    setRuns([]);
    setError("");
    if (!sessionId) return () => { epochRef.current += 1; };
    setLoading(true);
    void client.listRuns(taskId, sessionId)
      .then((loaded) => {
        if (epoch === epochRef.current) setRuns(loaded);
      })
      .catch((reason) => {
        if (epoch === epochRef.current) {
          setError(reason instanceof Error ? reason.message : "读取运行历史失败");
        }
      })
      .finally(() => {
        if (epoch === epochRef.current) setLoading(false);
      });
    return () => {
      epochRef.current += 1;
    };
  }, [client, refreshVersion, sessionId, taskId]);

  if (!sessionId) return <p className="agent-side-empty">选择一个 PI 会话后查看运行历史。</p>;
  if (loading && runs.length === 0) {
    return <div className="agent-side-loading"><LoaderCircle className="spin" size={13} />读取运行历史…</div>;
  }
  if (error) return <p className="agent-side-error">{error}</p>;
  if (runs.length === 0) return <p className="agent-side-empty">当前会话尚无运行记录。</p>;

  return (
    <div className="agent-run-list">
      {runs.map((run) => (
        <article key={run.id}>
          <header><Activity size={11} /><strong>{run.state}</strong><span>{duration(run)}</span></header>
          <dl>
            <div><dt>Run</dt><dd>{run.id}</dd></div>
            <div><dt>模式</dt><dd>{run.mode}</dd></div>
            {run.baselineCommit && <div><dt>基线</dt><dd>{run.baselineCommit.slice(0, 10)}</dd></div>}
            <div><dt>开始</dt><dd>{run.startedAt}</dd></div>
          </dl>
          {run.resultSummary && <p>{run.resultSummary}</p>}
          {run.errorMessage && <p className="agent-side-error">{run.errorMessage}</p>}
        </article>
      ))}
    </div>
  );
}
