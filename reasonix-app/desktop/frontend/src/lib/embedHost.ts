// Embed 模式（BTask RX 完全嵌入）的 DOM 上下文工具。
//
// 嵌入形态：reasonix App 渲染在宿主（BTask）页面一个 div 的 shadow root 内，
// 与宿主共享同一个 document（非 iframe）。本模块提供"当前挂载上下文"——
// 一切需要 document 级查询/样式/焦点操作的代码，在 embed 模式下都必须走这里，
// 否则会命中主 document（找不到 shadow 内元素、污染宿主样式/属性）。

let primaryShadowRoot: ShadowRoot | null = null;
let shadowRoot: ShadowRoot | null = null;
const auxiliaryShadowRoots: ShadowRoot[] = [];

export const OPEN_HOST_REASONIX_SETTINGS_EVENT =
  "btask:open-reasonix-settings";
export const EMBEDDED_REASONIX_SETTINGS_CHANGED_EVENT =
  "btask:reasonix-settings-changed";

function prepareEmbedRoot(root: ShadowRoot): void {
  const host = root.host as HTMLElement;
  host.setAttribute("data-rx-embed", "true");
  // embedEntry 不经过 desktop main.tsx 的 initTheme。先补默认视觉方向，
  // 避免 --surface-* 未定义时 button 回退成系统白色；异步设置加载后会覆盖。
  if (!host.hasAttribute("data-theme-style")) {
    host.setAttribute("data-theme-style", "graphite");
  }
}

function refreshActiveShadowRoot(): void {
  shadowRoot = auxiliaryShadowRoots.at(-1) ?? primaryShadowRoot;
}

/** 由主工作区 embedEntry 在挂载/卸载时设置。 */
export function setEmbedShadowRoot(root: ShadowRoot | null): void {
  const previous = primaryShadowRoot;
  primaryShadowRoot = root;
  if (
    previous &&
    previous !== root &&
    !auxiliaryShadowRoots.includes(previous)
  ) {
    (previous.host as HTMLElement).removeAttribute("data-rx-embed");
  }
  if (root) prepareEmbedRoot(root);
  refreshActiveShadowRoot();
}

/**
 * 注册临时的嵌入界面（例如宿主设置页中的原生 RX 设置面板）。临时界面
 * 活跃期间成为 DOM 查询与主题操作目标，卸载后自动恢复主工作区根节点。
 */
export function registerAuxiliaryEmbedShadowRoot(root: ShadowRoot): () => void {
  auxiliaryShadowRoots.push(root);
  prepareEmbedRoot(root);
  refreshActiveShadowRoot();
  return () => {
    const index = auxiliaryShadowRoots.lastIndexOf(root);
    if (index >= 0) auxiliaryShadowRoots.splice(index, 1);
    if (root !== primaryShadowRoot && !auxiliaryShadowRoots.includes(root)) {
      (root.host as HTMLElement).removeAttribute("data-rx-embed");
    }
    refreshActiveShadowRoot();
  };
}

/** 当前是否处于 shadow 嵌入模式。 */
export function isEmbedded(): boolean {
  return shadowRoot !== null;
}

/**
 * 嵌入 BTask 时把设置导航交还宿主；独立 Reasonix 返回 false，继续打开
 * 自己的设置中心。
 */
export function requestHostReasonixSettings(): boolean {
  if (!isEmbedded() || typeof window === "undefined") return false;
  window.dispatchEvent(new CustomEvent(OPEN_HOST_REASONIX_SETTINGS_EVENT));
  return true;
}

/** shadow 内的文档（与主 document 相同对象，仅供类型一致）。 */
export function rxDocument(): Document {
  return (shadowRoot?.ownerDocument ?? document) as Document;
}

/** 优先在 shadow 内查询，回退主 document。 */
export function rxQuerySelector<T extends Element = Element>(selector: string): T | null {
  return (shadowRoot?.querySelector(selector) ?? document.querySelector(selector)) as T | null;
}

/** 优先在 shadow 内查询全部，回退主 document。 */
export function rxQuerySelectorAll<T extends Element = Element>(selector: string): NodeListOf<T> {
  if (shadowRoot) return shadowRoot.querySelectorAll<T>(selector);
  return document.querySelectorAll<T>(selector);
}

/** 优先在 shadow 内按 id 查询，回退主 document。 */
export function rxGetElementById<T extends Element = Element>(id: string): T | null {
  if (shadowRoot) {
    return shadowRoot.querySelector<T>(`#${CSS.escape(id)}`);
  }
  return document.getElementById(id) as T | null;
}

/** 主题/平台属性与 CSS 变量的挂载目标（shadow 模式为 host 元素）。 */
export function rxRootElement(): HTMLElement {
  if (shadowRoot) return shadowRoot.host as HTMLElement;
  return document.documentElement;
}

/** shadow 内"body"等价物：App 根容器；回退主 body。 */
export function rxBody(): HTMLElement {
  if (shadowRoot) {
    return (shadowRoot.querySelector<HTMLElement>(".rx-app-root") ?? (shadowRoot.host as HTMLElement));
  }
  return document.body;
}

/** 当前焦点元素（shadow 内焦点 retarget 到 host，必须查 shadowRoot.activeElement）。 */
export function rxActiveElement(): Element | null {
  if (shadowRoot) return shadowRoot.activeElement;
  return document.activeElement;
}

/** portal 落点：优先 .chat-pane，回退 shadow 根容器。 */
export function rxPortalTarget(): Element {
  return (rxQuerySelector(".chat-pane") ?? rxBody()) as Element;
}

/** 滚动锁（全屏 modal 时锁定 shadow 内滚动，不碰主页面）。 */
export function rxLockScroll(lock: boolean): void {
  const target = rxBody();
  if (target) target.style.overflow = lock ? "hidden" : "";
}

/** embed 模式直连调用走这里（window.go 在 embed 模式下未被删除）。 */
export function rxDirectCall(method: string, args: unknown[]): Promise<unknown> {
  const goApp = (window as unknown as { go?: { main?: { App?: Record<string, (...a: unknown[]) => unknown> } } }).go?.main?.App;
  const fn = goApp?.[method];
  if (typeof fn !== "function") {
    return Promise.reject(new Error(`Reasonix 宿主未提供绑定方法: ${method}`));
  }
  return Promise.resolve(fn(...args));
}

// rxKeyActive 判断键盘事件是否来自 Reasonix 界面（embed 模式）：
// shadow 内事件 retarget 到 host——主 document 按键 target 是宿主元素。
// 全局快捷键/输入处理在 embed 模式下用它守卫，避免污染宿主页面。
export function rxKeyActive(event: KeyboardEvent): boolean {
  if (!isEmbedded()) return true;
  if (shadowRoot === null) return true;
  const target = event.target as Node | null;
  return target === shadowRoot.host || (target?.nodeType === Node.ELEMENT_NODE && (shadowRoot.host as HTMLElement).contains(target));
}
