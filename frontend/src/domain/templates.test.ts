import { describe, expect, it } from "vitest";
import {
  createEmptyDevelopment,
  createEmptyRequirements,
  createEmptyReview,
  type Task,
} from "./task";
import { buildRequirementDraft } from "./templates";

function makeTask(): Task {
  return {
    id: "task-1",
    title: "修复菜单文字对齐",
    summary: "菜单中的一级、二级、三级文字需要垂直居中。",
    projectName: "blue7zz/Y16-app",
    priority: "medium",
    status: "requirements",
    evidence: [
      {
        id: "source-1",
        type: "chat",
        title: "用户原始说明",
        content: "一级、二级、三级菜单文字都需要垂直居中。\n不得修改其他页面。",
        createdAt: "2026-07-25T00:00:00.000Z",
      },
    ],
    requirements: {
      ...createEmptyRequirements(),
      objective: "让菜单文字垂直居中",
      scope: ["一级、二级、三级菜单"],
      outOfScope: ["其他页面"],
      acceptanceCriteria: ["三个层级的文字均肉眼垂直居中"],
    },
    development: createEmptyDevelopment(),
    review: createEmptyReview(),
    revision: 2,
    createdAt: "2026-07-25T00:00:00.000Z",
    updatedAt: "2026-07-25T00:00:00.000Z",
  };
}

describe("buildRequirementDraft", () => {
  it("keeps a traceable source marker next to copied facts", () => {
    const draft = buildRequirementDraft(makeTask());
    expect(draft.document).toContain("[S1]");
    expect(draft.document).toContain("用户原始说明");
    expect(draft.facts[0].sourceId).toBe("source-1");
  });

  it("does not invent missing scope or acceptance criteria", () => {
    const task = makeTask();
    task.requirements.scope = [];
    task.requirements.acceptanceCriteria = [];
    const draft = buildRequirementDraft(task);

    expect(draft.scope).toEqual([]);
    expect(draft.acceptanceCriteria).toEqual([]);
    expect(
      draft.questions.some((question) =>
        question.question.includes("客观判定"),
      ),
    ).toBe(true);
  });

  it("puts explicit stop conditions into the execution prompt", () => {
    const draft = buildRequirementDraft(makeTask());
    expect(draft.executionPrompt).toContain("严禁推测");
    expect(draft.executionPrompt).toContain("立即停止并提出问题");
  });
});

