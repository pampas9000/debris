import tailwindcss from "@tailwindcss/vite";
import solid from "@solidjs/vite-plugin";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [tailwindcss(), solid()],
  server: {
    port: 5173,
    strictPort: true,
  },
  build: {
    target: "esnext",
  },
});
