import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [tailwindcss()],
  build: {
    outDir: "dist",
    // payload.json is fetched at runtime (never bundled); everything else is
    // a tiny SPA, so keep default chunking.
  },
});
