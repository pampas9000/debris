import { defineConfig } from "mygo-cli";

const version = (process.env.DEBRIS_VERSION || "0.1.0").replace(/^v/, "");

export default defineConfig({
  name: "Debris",
  identifier: "dev.debris.gui",
  version,
  out: "build",
  bindings: "mygo.ts",
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  linux: {
    command: "debris-gui",
    comment: "Bridge the gap between design library formats",
    categories: ["Utility", "Graphics"],
  },
});
