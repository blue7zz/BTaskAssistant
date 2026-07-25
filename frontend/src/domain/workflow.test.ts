import { describe, expect, it } from "vitest";
import { validateTransition } from "./workflow";

const closedGates = {
  requirementsConfirmed: false,
  developmentCompleted: false,
  reviewApproved: false,
};

describe("validateTransition", () => {
  it("allows only one manual stage at a time", () => {
    expect(validateTransition("inbox", "approved", closedGates)).toContain(
      "一个阶段",
    );
  });

  it("blocks development readiness until a human confirms requirements", () => {
    expect(
      validateTransition("requirements", "approved", closedGates),
    ).toContain("人工确认");
  });

  it("allows a confirmed requirement to move forward", () => {
    expect(
      validateTransition("requirements", "approved", {
        ...closedGates,
        requirementsConfirmed: true,
      }),
    ).toBeNull();
  });

  it("blocks completion until review is approved", () => {
    expect(validateTransition("review", "done", closedGates)).toContain(
      "审核",
    );
  });

  it("allows a one-step manual rollback without inventing a gate", () => {
    expect(validateTransition("review", "development", closedGates)).toBeNull();
  });
});

