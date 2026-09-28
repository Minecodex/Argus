import { webVendorChunk } from "../../../scripts/web-chunks";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: { port: 4173, strictPort: true },
  preview: { port: 4173, strictPort: true },
  build: {
    manifest: true,
    rollupOptions: {
      output: {
        onlyExplicitManualChunks: true,
        manualChunks: webVendorChunk,
      },
    },
  },
});
