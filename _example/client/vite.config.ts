import { defineConfig } from "vite";

export default defineConfig({
  server: {
    port: 5173,
    host: true,
    proxy: {
      "/rpc": {
        target: "http://localhost:8081",
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/rpc/, "/"),
      },
    },
  },
});
