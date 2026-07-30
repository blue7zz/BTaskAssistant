import { describe, expect, it } from "vitest";
import {
  buildRequirementAnalysisInput,
  isApprovalBlockingQuestion,
  normalizeRequirements,
} from "./requirements";
import {
  createEmptyDevelopment,
  createEmptyRequirements,
  createEmptyReview,
  type Task,
} from "./task";

function makeTask(): Task {
  return {
    id: "task-1",
    title: "需求访谈",
    summary: "整理需求并发现信息缺口。",
    projectName: "BTaskAssistant",
    projectPath: "/projects/BTaskAssistant",
    priority: "medium",
    status: "requirements",
    evidence: [
      {
        id: "source-1",
        type: "manual",
        title: "参与分析",
        content: "需求必须人工批准。",
        createdAt: "2026-07-28T00:00:00Z",
        selectedForAnalysis: true,
      },
      {
        id: "source-2",
        type: "chat",
        title: "已排除",
        content: "这条资料不应进入输入包。",
        createdAt: "2026-07-28T00:00:00Z",
        selectedForAnalysis: false,
      },
    ],
    requirements: {
      ...createEmptyRequirements(),
      questions: [
        {
          id: "question-1",
          question: "范围是什么？",
          answer: "只做需求整理",
          status: "ANSWERED",
          severity: "BLOCKING",
          resolvedAt: "2026-07-28T01:00:00Z",
        },
        {
          id: "question-2",
          question: "是否需要动画？",
          answer: "",
          status: "OPEN",
          severity: "OPTIONAL",
        },
      ],
    },
    development: createEmptyDevelopment(),
    review: createEmptyReview(),
    revision: 3,
    createdAt: "2026-07-28T00:00:00Z",
    updatedAt: "2026-07-28T00:00:00Z",
  };
}

describe("requirement analysis input", () => {
  it("includes only selected materials and separates previous answers", () => {
    const input = buildRequirementAnalysisInput(makeTask());
    expect(input.materials.map((material) => material.materialId)).toEqual([
      "source-1",
    ]);
    expect(input.previousAnswers[0].questionId).toBe("question-1");
    expect(input.openQuestions[0].id).toBe("question-2");
    expect(input.analysisPolicy.allowCodeWrite).toBe(false);
  });

  it("keeps migrated questions blocking by default", () => {
    const requirements = normalizeRequirements({
      questions: [{ id: "legacy", question: "旧问题", answer: "" }],
    });
    expect(requirements.questions[0].severity).toBe("BLOCKING");
    expect(isApprovalBlockingQuestion(requirements.questions[0])).toBe(true);
  });
});
