import { webVendorChunk } from "../../../scripts/web-chunks";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: { port: 4174, strictPort: true },
  preview: { port: 4174, strictPort: true },
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
