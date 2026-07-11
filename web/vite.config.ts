import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { sites } from "./build/sites-vite-plugin";

const daemonProxy = {
  "/ws": {
    target: "ws://127.0.0.1:7331",
    ws: true,
    changeOrigin: false,
  },
  "/terminal": {
    target: "ws://127.0.0.1:7331",
    ws: true,
    changeOrigin: false,
  },
  "/healthz": {
    target: "http://127.0.0.1:7331",
    changeOrigin: false,
  },
  "/attachments": {
    target: "http://127.0.0.1:7331",
    changeOrigin: false,
  },
  "/project/server": {
    target: "http://127.0.0.1:7331",
    ws: true,
    changeOrigin: false,
  },
};

export default defineConfig({
  plugins: [react(), sites()],
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: daemonProxy,
  },
  preview: {
    host: "127.0.0.1",
    port: 4173,
    proxy: daemonProxy,
  },
});
