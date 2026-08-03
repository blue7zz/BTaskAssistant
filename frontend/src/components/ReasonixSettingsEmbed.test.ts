import { afterEach, describe, expect, it } from "vitest";
import { mountReasonixSettingsEmbed } from "../../../reasonix-app/desktop/frontend/src/settingsEmbedEntry";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = false;

describe("Reasonix settings embed", () => {
  let host: HTMLDivElement | undefined;
  let unmount: (() => void) | undefined;

  afterEach(() => {
    unmount?.();
    unmount = undefined;
    host?.remove();
    host = undefined;
  });

  it("在独立 Shadow DOM 中渲染 RX 原生设置中心", async () => {
    host = document.createElement("div");
    document.body.appendChild(host);

    unmount = mountReasonixSettingsEmbed(host);
    await new Promise((resolve) => setTimeout(resolve, 0));

    const shadow = host.shadowRoot;
    expect(shadow).not.toBeNull();
    expect(shadow?.querySelector(".settings-modal--inline")).not.toBeNull();
    expect(shadow?.querySelector(".settings-center")).not.toBeNull();
    expect(
      Array.from(shadow?.querySelectorAll(".settings-center__navitem") ?? [])
        .map((item) => item.textContent)
        .join(" "),
    ).toContain("General");
    expect(shadow?.querySelector(".modal-close-button")).toBeNull();
  });
});
