import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { webVendorChunk } from "../../../scripts/web-chunks";
export default defineConfig({
  plugins: [tailwindcss()],
  server: { port: 4176, strictPort: true, cors: true },
  preview: { port: 4176, strictPort: true, cors: true },
  build: {
    manifest: true,
    rollupOptions: {
      output: {
        manualChunks: (id) => {
          const normalized = id.replaceAll("\\", "/");
          if (
            normalized.endsWith(
              "/web/packages/ui/src/template-styles.css?inline",
            )
          )
            return "template-shared-styles";
          if (
            normalized.endsWith(
              "/web/packages/ui/src/template-foundation.css?inline",
            )
          )
            return "template-foundation";
          return webVendorChunk(id);
        },
      },
    },
  },
});
