import { defineConfig } from "vite";
export default defineConfig({
  server: { port: 4176, strictPort: true, cors: true },
  preview: { port: 4176, strictPort: true, cors: true },
  build: { manifest: true },
});
