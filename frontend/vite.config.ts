/// <reference types="vitest/config" />
import tailwindcss from "@tailwindcss/vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { svelteTesting } from "@testing-library/svelte/vite";
import { defineConfig } from "vite";

// Proxy API and health routes to the Go backend during development so the SPA
// and API share an origin (matching production, where the API embeds the SPA).
const backend = "http://localhost:8080";

export default defineConfig({
  plugins: [svelte(), tailwindcss(), svelteTesting()],
  // Bake the build-time VERSION (set by env.bash / the Docker build) into the
  // bundle so the footer can show which build is running. Falls back to "dev"
  // for plain `vite` runs that don't source the env.
  define: {
    __APP_VERSION__: JSON.stringify(process.env.VERSION ?? "dev"),
  },
  server: {
    proxy: {
      "/api": backend,
      "/livez": backend,
      "/readyz": backend,
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest-setup.ts"],
    include: ["src/**/*.{test,spec}.{ts,js}"],
  },
});
