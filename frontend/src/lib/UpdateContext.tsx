import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import type { UpdateState } from "../../bindings/veda/internal/model/models";
import { AppService } from "./api";

// 检查与下载都在 Go 侧完成，前端只需要轮询状态。空闲时低频确认，一旦发现
// 正在检查或下载就加快轮询，下载完成后停下，避免长期空转。
const UPDATE_IDLE_POLL = 10000;
const UPDATE_ACTIVE_POLL = 500;

type Ctx = {
  state: UpdateState | null;
  ready: boolean;
};

const UpdateContext = createContext<Ctx | null>(null);

export function UpdateProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<UpdateState | null>(null);

  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;

    const poll = async () => {
      let delay = UPDATE_IDLE_POLL;
      try {
        const value = await AppService.GetUpdateState();
        if (cancelled) {
          return;
        }
        if (value) {
          setState(value);
          // 安装包已经下载完，入口只需要出现一次，不需要继续轮询。
          if (value.status === "ready") {
            return;
          }
          if (value.status === "checking" || value.status === "downloading") {
            delay = UPDATE_ACTIVE_POLL;
          }
        }
      } catch {
        // 更新状态只用于展示入口，读取失败时等下一次轮询即可。
      }
      if (!cancelled) {
        timer = window.setTimeout(() => void poll(), delay);
      }
    };

    void poll();
    return () => {
      cancelled = true;
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, []);

  const ready = state?.status === "ready";
  const value = useMemo<Ctx>(() => ({ state, ready }), [state, ready]);

  return <UpdateContext.Provider value={value}>{children}</UpdateContext.Provider>;
}

export function useUpdate() {
  const ctx = useContext(UpdateContext);
  if (!ctx) {
    throw new Error("useUpdate must be used within UpdateProvider");
  }
  return ctx;
}
