import { defineConfig } from "vite";

export default defineConfig({
  resolve: {
    alias: {
      "@ui": "/src/ui",
      "@app": "/src/app",
      "@api": "/src/api",
      "@types": "/src/types",
      "@signaling": "/src/signaling",
    },
  },
  server: {
    port: 5173,
    host: true,
    proxy: {
      "/ws": {
        target: "ws://localhost:8081",
        changeOrigin: true,
        ws: true,
      },
      "/rpc": {
        target: "http://localhost:8081",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/rpc/, "/"),
      },
    },
  },
});
