import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    target: "es2022",
    outDir: "dist",
    // 动态 import chunk（Reasonix embed）的命名导出必须保留——
    // esbuild minify 默认会重命名导出标识符，导致宿主拿不到挂载函数。
    rollupOptions: {
      output: {
        minifyInternalExports: false,
      },
    },
    // The full Markdown editor is intentionally lazy-loaded as a separate
    // feature chunk; keep the initial application bundle below this budget.
    chunkSizeWarningLimit: 800,
  },
});
