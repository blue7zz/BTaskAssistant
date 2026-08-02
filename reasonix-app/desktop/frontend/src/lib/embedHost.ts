// Embed 模式（BTask RX 完全嵌入）的 DOM 上下文工具。
//
// 嵌入形态：reasonix App 渲染在宿主（BTask）页面一个 div 的 shadow root 内，
// 与宿主共享同一个 document（非 iframe）。本模块提供"当前挂载上下文"——
// 一切需要 document 级查询/样式/焦点操作的代码，在 embed 模式下都必须走这里，
// 否则会命中主 document（找不到 shadow 内元素、污染宿主样式/属性）。

let shadowRoot: ShadowRoot | null = null;

/** 由 embedEntry 在挂载/卸载时设置。 */
export function setEmbedShadowRoot(root: ShadowRoot | null): void {
  shadowRoot = root;
}

/** 当前是否处于 shadow 嵌入模式。 */
export function isEmbedded(): boolean {
  return shadowRoot !== null;
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
