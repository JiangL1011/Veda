import * as AppService from "../../bindings/veda/appservice";
import type { OpenTarget } from "../../bindings/veda/models";

export { AppService };
export type Workspace = Pick<OpenTarget, "kind" | "path" | "file">;

export function asWorkspace(value: unknown): Workspace | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const rec = value as Record<string, unknown>;
  const kind = String(rec.kind ?? "");
  const path = String(rec.path ?? "");
  if ((kind !== "file" && kind !== "directory") || !path) {
    return null;
  }
  const file = rec.file ? String(rec.file) : undefined;
  return { kind, path, file };
}
