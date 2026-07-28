import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [tailwindcss()],
  build: {
    outDir: "dist",
    // 默认清空 dist 再写；受限沙箱（bulk-delete 守护）下用 VITE_NO_EMPTY=1
    // 跳过清空，改为增量覆盖（CI/Cloudflare 每次全新 checkout，不受影响）。
    emptyOutDir: process.env.VITE_NO_EMPTY !== "1",
    // payload.json is fetched at runtime (never bundled); everything else is
    // a tiny SPA, so keep default chunking.
  },
});
