import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import svelte from "eslint-plugin-svelte";
import { defineConfig, globalIgnores } from "eslint/config";
import svelteConfig from "./svelte.config.js";

export default defineConfig([
  globalIgnores(["dist", "build", "node_modules"]),
  {
    files: ["**/*.{js,mjs,cjs,ts,mts,cts,svelte,svelte.ts,svelte.js}"],
    plugins: { js },
    extends: ["js/recommended"],
    // __APP_VERSION__ is a compile-time constant injected by Vite's `define`.
    languageOptions: { globals: { ...globals.browser, __APP_VERSION__: "readonly" } },
  },
  {
    files: ["**/*.{ts,mts,cts,svelte,svelte.ts}"],
    extends: [tseslint.configs.recommendedTypeChecked],
    languageOptions: {
      parserOptions: {
        projectService: true,
      },
    },
  },
  svelte.configs.recommended,
  {
    files: ["**/*.svelte", "**/*.svelte.ts", "**/*.svelte.js"],
    languageOptions: {
      parserOptions: {
        projectService: true,
        extraFileExtensions: [".svelte"],
        parser: tseslint.parser,
        svelteConfig,
      },
    },
    rules: {
      "svelte/button-has-type": "error",
      "svelte/no-conflicting-module-names": "error",
      "svelte/no-ignored-unsubscribe": "error",
      "svelte/no-nested-style-tag": "error",
      "svelte/no-target-blank": "error",
      "svelte/require-store-callbacks-use-set-param": "error",
      "svelte/valid-compile": "error",
      "svelte/valid-style-parse": "error",
    },
  },
]);
