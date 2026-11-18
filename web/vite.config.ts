/// <reference types="vitest/config" />
import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

// The page is built into the folder awkit embeds (internal/workbench/dist).
// While it is worked on, `npm run dev` serves it and hands /api to an
// `awkit ui` that is running (AWKIT_UI, http://127.0.0.1:4747 by default):
// open that awkit's address once, so that its cookie is set.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  build: {
    outDir: fileURLToPath(new URL("../internal/workbench/dist", import.meta.url)),
    emptyOutDir: true,
    chunkSizeWarningLimit: 900,
  },
  server: {
    proxy: { "/api": { target: process.env.AWKIT_UI ?? "http://127.0.0.1:4747", changeOrigin: true } },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
