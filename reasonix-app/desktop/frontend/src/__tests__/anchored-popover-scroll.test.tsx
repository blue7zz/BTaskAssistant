// Run: tsx src/__tests__/anchored-popover-scroll.test.tsx

import { JSDOM } from "jsdom";
import React, { useRef } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { AnchoredPopover } from "../components/AnchoredPopover";
import { setEmbedShadowRoot } from "../lib/embedHost";

type RectParts = Pick<DOMRect, "left" | "top" | "right" | "bottom" | "width" | "height">;

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function eq(actual: unknown, expected: unknown, label: string) {
  if (actual === expected) {
    ok(true, label);
  } else {
    ok(false, `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
  }
}

function rect({ left, top, right, bottom, width, height }: RectParts): DOMRect {
  return {
    left,
    top,
    right,
    bottom,
    width,
    height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect;
}

async function nextFrame() {
  await act(async () => {
    await new Promise((resolve) => requestAnimationFrame(() => resolve(undefined)));
  });
}

function Harness({ onClose = () => {} }: { onClose?: () => void }) {
  const anchorRef = useRef<HTMLButtonElement>(null);
  return (
    <>
      <button ref={anchorRef} data-testid="anchor">Anchor</button>
      <AnchoredPopover
        open
        anchorRef={anchorRef}
        onClose={onClose}
        className="test-popover"
        placement="bottom"
      >
        <div>Menu</div>
      </AnchoredPopover>
    </>
  );
}

console.log("\nanchored popover scroll positioning");

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
  pretendToBeVisual: true,
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.Node = dom.window.Node;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Event = dom.window.Event;
globalThis.KeyboardEvent = dom.window.KeyboardEvent;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);

Object.defineProperty(window, "innerWidth", { configurable: true, value: 800 });
Object.defineProperty(window, "innerHeight", { configurable: true, value: 600 });

let anchorTop = 100;
const originalGetBoundingClientRect = dom.window.HTMLElement.prototype.getBoundingClientRect;
dom.window.HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
  if (this instanceof dom.window.HTMLElement && this.dataset.testid === "anchor") {
    return rect({ left: 60, top: anchorTop, right: 260, bottom: anchorTop + 30, width: 200, height: 30 });
  }
  if (this instanceof dom.window.HTMLElement && this.dataset.anchoredPopover === "active") {
    return rect({ left: 0, top: 0, right: 240, bottom: 120, width: 240, height: 120 });
  }
  return originalGetBoundingClientRect.call(this);
};

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);

await act(async () => {
  root.render(<Harness />);
});
await nextFrame();

const popover = document.querySelector<HTMLElement>("[data-anchored-popover='active']");
if (!popover) throw new Error("popover did not render");

eq(popover.style.top, "138px", "popover starts below the anchor");

await act(async () => {
  anchorTop = 40;
  window.dispatchEvent(new Event("scroll", { bubbles: true }));
  await new Promise((resolve) => requestAnimationFrame(() => resolve(undefined)));
});

eq(popover.style.top, "78px", "popover follows the anchor after a scroll event");

await act(async () => {
  root.unmount();
});

const embedHost = document.createElement("div");
document.body.appendChild(embedHost);
const embedShadow = embedHost.attachShadow({ mode: "open" });
embedShadow.innerHTML = '<div class="rx-app-root"><div class="chat-pane"><div id="embed-root"></div></div></div>';
setEmbedShadowRoot(embedShadow);

let embeddedCloseCalls = 0;
const embedRootEl = embedShadow.querySelector<HTMLElement>("#embed-root");
if (!embedRootEl) throw new Error("missing embedded root");
const embedRoot = createRoot(embedRootEl);

await act(async () => {
  embedRoot.render(<Harness onClose={() => { embeddedCloseCalls += 1; }} />);
});
await nextFrame();

const embeddedAnchor = embedShadow.querySelector<HTMLElement>("[data-testid='anchor']");
if (!embeddedAnchor) throw new Error("embedded anchor did not render");

await act(async () => {
  embeddedAnchor.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, composed: true }));
});

eq(embeddedCloseCalls, 0, "shadow-dom anchor click is not mistaken for an outside click");

await act(async () => {
  document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, composed: true }));
});

eq(embeddedCloseCalls, 1, "a real click outside the embedded popover closes it");

await act(async () => {
  embedRoot.unmount();
});
setEmbedShadowRoot(null);
embedHost.remove();
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
