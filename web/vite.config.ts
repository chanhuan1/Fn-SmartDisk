import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  base: "./",
  build: {
    outDir: "../server/web/dist",
    emptyOutDir: true,
    assetsInlineLimit: 4096,
    chunkSizeWarningLimit: 900,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:18315",
        changeOrigin: true,
      },
    },
  },
});
