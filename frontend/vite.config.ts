import { cpSync, createReadStream, existsSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { defineConfig, type Plugin } from "vite";

const root = path.dirname(fileURLToPath(import.meta.url));
const vditorDist = path.resolve(root, "node_modules/vditor/dist");
const vditorUrlPrefix = "/vditor/dist/";

const MIME: Record<string, string> = {
  ".css": "text/css; charset=utf-8",
  ".gif": "image/gif",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".map": "application/json; charset=utf-8",
  ".png": "image/png",
  ".svg": "image/svg+xml",
  ".ttf": "font/ttf",
  ".wasm": "application/wasm",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".webp": "image/webp",
};

function copyVditorDist(dest: string) {
  if (!existsSync(vditorDist)) {
    throw new Error("vditor dist not found; run npm install in frontend");
  }
  cpSync(vditorDist, dest, { recursive: true });
}

function vditorAssets(): Plugin {
  return {
    name: "vditor-assets",
    buildStart() {
      copyVditorDist(path.resolve(root, "public/vditor/dist"));
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = req.url?.split("?")[0] ?? "";
        if (!url.startsWith(vditorUrlPrefix)) {
          next();
          return;
        }
        const rel = decodeURIComponent(url.slice(vditorUrlPrefix.length));
        const file = path.resolve(vditorDist, rel);
        if (!file.startsWith(vditorDist) || !existsSync(file) || !statSync(file).isFile()) {
          next();
          return;
        }
        const ext = path.extname(file).toLowerCase();
        res.setHeader("Content-Type", MIME[ext] || "application/octet-stream");
        res.setHeader("Cache-Control", "no-cache");
        createReadStream(file).pipe(res);
      });
    },
    closeBundle() {
      copyVditorDist(path.resolve(root, "dist/vditor/dist"));
    },
  };
}

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  optimizeDeps: {
    include: ["vditor"],
  },
  plugins: [react(), tailwindcss(), wails("./bindings"), vditorAssets()],
});
