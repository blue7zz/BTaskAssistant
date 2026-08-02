/*
 * Terminal panel adapted from Reasonix (rightDock.terminal). Commands run
 * through the task session's bash RPC. Reasonix is MIT licensed.
 */

import { Square, TerminalSquare } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { AgentClient } from "../lib/agentBridge";

interface TerminalRun {
  id: string;
  command: string;
  output: string;
  status: "running" | "done" | "failed" | "cancelled";
  exitCode?: number;
}

interface AgentTerminalPanelProps {
  taskId: string;
  sessionId: string;
  client: AgentClient;
  onError(message: string): void;
}

export function AgentTerminalPanel({
  taskId,
  sessionId,
  client,
  onError,
}: AgentTerminalPanelProps) {
  const [runs, setRuns] = useState<TerminalRun[]>([]);
  const [command, setCommand] = useState("");
  const [busy, setBusy] = useState(false);
  const outputRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const output = outputRef.current;
    if (output) output.scrollTop = output.scrollHeight;
  }, [runs]);

  const run = async () => {
    const text = command.trim();
    if (!text || busy || !client.sessionCommand) return;
    setCommand("");
    setBusy(true);
    const runID = `bash-${Date.now().toString(36)}`;
    setRuns((current) => [
      ...current,
      { id: runID, command: text, output: "", status: "running" },
    ]);
    try {
      const result = (await client.sessionCommand({
        taskId,
        sessionId,
        type: "bash",
        payload: { command: text },
      })) as {
        output?: string;
        exitCode?: number;
        cancelled?: boolean;
        truncated?: boolean;
      };
      setRuns((current) =>
        current.map((item) =>
          item.id === runID
            ? {
                ...item,
                output: result.output ?? "",
                status: result.cancelled
                  ? "cancelled"
                  : (result.exitCode ?? 0) === 0
                    ? "done"
                    : "failed",
                exitCode: result.exitCode,
              }
            : item,
        ),
      );
    } catch (reason) {
      setRuns((current) =>
        current.map((item) =>
          item.id === runID
            ? {
                ...item,
                status: "failed",
                output: reason instanceof Error ? reason.message : "命令执行失败",
              }
            : item,
        ),
      );
      onError(reason instanceof Error ? reason.message : "命令执行失败");
    } finally {
      setBusy(false);
    }
  };

  const abort = async () => {
    if (!client.sessionCommand) return;
    try {
      await client.sessionCommand({
        taskId,
        sessionId,
        type: "abort_bash",
        payload: {},
      });
    } catch (reason) {
      onError(reason instanceof Error ? reason.message : "中止失败");
    }
  };

  return (
    <div className="agent-terminal-panel">
      <div className="agent-terminal-runs" ref={outputRef}>
        {runs.length === 0 && (
          <div className="agent-terminal-empty">运行命令查看输出</div>
        )}
        {runs.map((runItem) => (
          <div className="agent-terminal-run" key={runItem.id}>
            <div className="agent-terminal-command">
              <TerminalSquare size={12} />
              <code>{runItem.command}</code>
              <span className={`agent-terminal-state ${runItem.status}`}>
                {runItem.status === "running"
                  ? "运行中"
                  : runItem.status === "cancelled"
                    ? "已中止"
                    : runItem.status === "failed"
                      ? `失败${runItem.exitCode !== undefined ? ` (${runItem.exitCode})` : ""}`
                      : "完成"}
              </span>
            </div>
            {runItem.output && (
              <pre className="agent-terminal-output">{runItem.output}</pre>
            )}
          </div>
        ))}
      </div>
      <div className="agent-terminal-input">
        <input
          value={command}
          placeholder="输入命令…（在当前任务工作区执行）"
          aria-label="终端命令"
          disabled={busy || !client.sessionCommand}
          onChange={(event) => setCommand(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") void run();
          }}
        />
        {busy ? (
          <button
            type="button"
            className="agent-terminal-stop"
            aria-label="中止命令"
            title="中止命令"
            onClick={() => void abort()}
          >
            <Square size={12} fill="currentColor" />
          </button>
        ) : (
          <button
            type="button"
            className="button primary compact"
            disabled={!command.trim()}
            onClick={() => void run()}
          >
            运行
          </button>
        )}
      </div>
    </div>
  );
}
