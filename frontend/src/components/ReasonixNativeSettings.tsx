import { LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";

type MountReasonixSettingsEmbed = (
  host: HTMLElement,
  options?: { onMounted?: () => void },
) => () => void;

interface SettingsEmbedModule {
  mountReasonixSettingsEmbed?: MountReasonixSettingsEmbed;
  default?: MountReasonixSettingsEmbed;
}

let settingsEmbedModulePromise: Promise<SettingsEmbedModule> | undefined;

function loadSettingsEmbed(): Promise<SettingsEmbedModule> {
  settingsEmbedModulePromise ??= import(
    "../../../reasonix-app/desktop/frontend/src/settingsEmbedEntry"
  ) as Promise<SettingsEmbedModule>;
  return settingsEmbedModulePromise;
}

export function ReasonixNativeSettings() {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const [mounted, setMounted] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return undefined;
    let disposed = false;
    let unmount: (() => void) | undefined;

    void loadSettingsEmbed()
      .then((module) => {
        if (disposed) return;
        const mount = module.mountReasonixSettingsEmbed ?? module.default;
        if (typeof mount !== "function") {
          throw new Error("Reasonix 设置面板模块导出缺失");
        }
        unmount = mount(host, {
          onMounted: () => {
            if (!disposed) setMounted(true);
          },
        });
      })
      .catch((reason: unknown) => {
        if (disposed) return;
        setError(reason instanceof Error ? reason.message : String(reason));
      });

    return () => {
      disposed = true;
      unmount?.();
    };
  }, []);

  return (
    <div className="reasonix-native-settings">
      {!mounted && !error && (
        <div className="reasonix-native-settings-status" role="status">
          <LoaderCircle className="spin" size={15} />
          正在加载 RX 设置面板……
        </div>
      )}
      {error && (
        <div className="reasonix-native-settings-status error" role="alert">
          RX 设置面板加载失败：{error}
        </div>
      )}
      <div
        ref={hostRef}
        className="reasonix-native-settings-host"
        data-testid="reasonix-native-settings-host"
      />
    </div>
  );
}
