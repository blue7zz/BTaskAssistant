import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    target: "es2022",
    outDir: "dist",
    // The full Markdown editor is intentionally lazy-loaded as a separate
    // feature chunk; keep the initial application bundle below this budget.
    chunkSizeWarningLimit: 800,
  },
});
