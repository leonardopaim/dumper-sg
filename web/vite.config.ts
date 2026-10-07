import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
export default defineConfig({
  plugins: [
    react(),
    {
      name: "preserve-embed-marker",
      closeBundle() {
        writeFileSync(resolve(import.meta.dirname, "dist/.gitkeep"), "");
      },
    },
  ],
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: { "/api": "http://127.0.0.1:8787" },
  },
  preview: { host: "127.0.0.1", port: 5173, strictPort: true },
});
