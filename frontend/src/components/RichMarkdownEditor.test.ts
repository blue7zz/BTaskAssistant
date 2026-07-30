import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RichMarkdownEditor } from "./RichMarkdownEditor";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

describe("RichMarkdownEditor", () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  it("renders fenced code and the content that follows it in read-only mode", async () => {
    await act(async () => {
      root.render(
        createElement(RichMarkdownEditor, {
          value:
            "**需求描述**\n\n- 需求背景：\n\n```markdown\n目前有两个问题。\n```\n\n**验收标准**\n\n删除成功",
          disabled: true,
          onCommit: vi.fn(),
        }),
      );
    });

    expect(container.textContent).toContain("目前有两个问题。");
    expect(container.textContent).toContain("验收标准");
    expect(container.textContent).toContain("删除成功");
  });
});
