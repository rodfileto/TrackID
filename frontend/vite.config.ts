import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import svgr from "vite-plugin-svgr";

// MapLibre GL 6 starts its Web Worker from a URL it computes at run time next to
// its own module, which neither Vite's dependency pre-bundling (dev) nor the
// production bundle preserves. This serves the worker and the module it shares
// with the main library at a fixed path, /maplibre/; the map points MapLibre at
// it with setWorkerUrl (components/intelligence/OccurrenceMapView.tsx).
const maplibreDist = dirname(createRequire(import.meta.url).resolve("maplibre-gl/package.json")) + "/dist";
const maplibreWorkerFiles = ["maplibre-gl-worker.mjs", "maplibre-gl-shared.mjs"];

function maplibreWorker(): Plugin {
  return {
    name: "maplibre-worker",
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const name = req.url?.match(/^\/maplibre\/([\w.-]+\.mjs)$/)?.[1];
        if (!name || !maplibreWorkerFiles.includes(name)) return next();
        res.setHeader("Content-Type", "text/javascript");
        res.end(readFileSync(join(maplibreDist, name)));
      });
    },
    generateBundle() {
      for (const name of maplibreWorkerFiles) {
        this.emitFile({ type: "asset", fileName: `maplibre/${name}`, source: readFileSync(join(maplibreDist, name)) });
      }
    },
  };
}

// The application opens with no signal (docs/scene-examination-plan.md): a service worker, written
// here for each build, keeps the build's own files. A page asked for with no network gets the
// kept index.html, and the API is never kept: only the field notes work offline, from what the
// browser stores for them (src/offline/fieldNotesStore.ts).
function offlineShell(): Plugin {
  return {
    name: "offline-shell",
    apply: "build",
    enforce: "post",
    generateBundle(_, bundle) {
      const files = Object.keys(bundle)
        .filter((name) => /\.(js|mjs|css|woff2?|svg|png)$/.test(name) && !name.endsWith(".map"))
        .map((name) => "/" + name)
        .sort();
      files.push("/favicon.svg");
      const version = createHash("sha256").update(files.join("\n")).digest("hex").slice(0, 16);
      const source = readFileSync(join(import.meta.dirname, "src/offline/sw.template.js"), "utf8")
        .replace("__VERSION__", version)
        .replace("__FILES__", JSON.stringify(files));
      this.emitFile({ type: "asset", fileName: "sw.js", source });
    },
  };
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    react(),
    maplibreWorker(),
    offlineShell(),
    svgr({
      svgrOptions: {
        icon: true,
        // This will transform your SVG to a React component
        exportType: "named",
        namedExport: "ReactComponent",
      },
    }),
  ],
  server: {
    proxy: {
      // Listed before /api, which would otherwise match it. The Host header stays as the browser
      // sent it, because the API accepts WebSockets only when Origin matches Host.
      "/api/realtime": {
        target: "http://localhost:8082",
        ws: true,
      },
      "/api": {
        target: "http://localhost:8082",
        changeOrigin: true,
      },
      // The report editor (WordCraft's web build), served by the API so it shares this origin.
      "/wordcraft": {
        target: "http://localhost:8082",
        changeOrigin: true,
      },
    },
  },
});
