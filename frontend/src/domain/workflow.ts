import { statusIndex, type TaskStatus } from "./task";

export interface TransitionGates {
  requirementsConfirmed: boolean;
  developmentCompleted: boolean;
  reviewApproved: boolean;
}

export function validateTransition(
  from: TaskStatus,
  to: TaskStatus,
  gates: TransitionGates,
): string | null {
  const difference = statusIndex(to) - statusIndex(from);

  if (difference === 0) return null;
  if (difference > 1 || difference < -1) {
    return "任务只能手动推进或退回一个阶段";
  }
  if (difference < 0) return null;

  if (to === "approved" && !gates.requirementsConfirmed) {
    return "需求尚未人工确认，不能进入待开发";
  }
  if (to === "development" && !gates.requirementsConfirmed) {
    return "需求确认已失效，不能开始开发";
  }
  if (to === "review" && !gates.developmentCompleted) {
    return "尚未记录开发完成结果，不能进入审核";
  }
  if (to === "done" && !gates.reviewApproved) {
    return "审核尚未人工通过，不能完成任务";
  }
  return null;
}

