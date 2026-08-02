type ObjectConstructorWithHasOwn = ObjectConstructor & {
  hasOwn?: (value: object, property: PropertyKey) => boolean;
};

// Safari 15.3 / the matching macOS WebKit used by older Wails shells predates
// Object.hasOwn. react-markdown 10 calls it while validating deprecated props,
// so install the tiny ES2022 primitive before React imports the markdown chunk.
export function installObjectHasOwnPolyfill(target: ObjectConstructorWithHasOwn = Object): void {
  if (typeof target.hasOwn === "function") return;
  Object.defineProperty(target, "hasOwn", {
    configurable: true,
    writable: true,
    value(value: object, property: PropertyKey) {
      return Object.prototype.hasOwnProperty.call(value, property);
    },
  });
}

installObjectHasOwnPolyfill();

// Host-embedding escape hatch (e.g. the BTaskAssistant RX tab): inside a host
// app's iframe, Wails injects a PARTIAL window.go / window.runtime — the objects
// exist but the binding methods are absent — so Reasonix would mistake the
// environment for the native shell and every bridge call fails
// ("Lc.Platform is not a function"). When the host loads us with ?browser=1,
// delete both globals before any other module evaluates so every bridge
const rxHostParams =
  typeof window === "undefined"
    ? null
    : new URLSearchParams(window.location?.search ?? "");
if (
  rxHostParams !== null &&
  (rxHostParams.has("browser") || rxHostParams.has("host"))
) {
  delete (window as { go?: unknown }).go;
  delete (window as { runtime?: unknown }).runtime;
}
