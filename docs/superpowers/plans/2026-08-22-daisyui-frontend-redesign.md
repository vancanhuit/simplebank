# daisyUI Frontend Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the complete SimpleBank SPA with daisyUI 5, custom persisted light/dark themes, and improved responsive UX without changing any banking behavior or API contract.

**Architecture:** daisyUI owns component appearance and semantic theme tokens, Tailwind utilities own layout and responsive composition, and the existing Svelte components retain application behavior and accessible semantics. A small framework-independent theme module initializes the document before mount; a focused Svelte toggle component is its only UI adapter.

**Tech Stack:** Svelte 5, TypeScript 6, Vite 8, Tailwind CSS 4, daisyUI 5, Vitest, Testing Library, Playwright, axe-core, Bun, Go embedded SPA assets

**Spec:** `docs/superpowers/specs/2026-08-22-daisyui-frontend-redesign-design.md`

## Global Constraints

- This is a frontend-only UX change; do not change backend APIs, domain rules, authentication, authorization, transfer semantics, or persisted banking data.
- Preserve authentication initialization and redirects, account reset generation checks, logout cleanup, transfer idempotency, API-driven transfer and account-opening limits, verification credential cleanup, and SPA route focus announcements.
- Use exactly two custom themes named `simplebank-light` and `simplebank-dark`.
- Initialize the selected theme before mounting Svelte; accept only supported names, fall back to OS preference, persist explicit changes, and tolerate unavailable storage.
- Use daisyUI component classes for component appearance and Tailwind utilities for layout; do not build a parallel component styling system.
- Keep native semantic elements and all existing labels, live-region roles, keyboard behavior, minimum touch targets, reduced-motion behavior, and forced-color usability.
- Preserve the IBM Plex Sans Variable local font and tabular numerals for monetary values.
- Support the existing tested viewport range from 320px through 1440px without horizontal overflow.
- Do not edit generated backend code or alter the SPA embedding order.
- Use the pinned `mise`/Bun toolchain and update `frontend/package.json` and `frontend/bun.lock` together.

---

## File Structure

### New Files

- `frontend/src/lib/theme.ts`: Supported theme constants, validation, initial preference resolution, DOM application, persistence, and toggle helpers.
- `frontend/src/lib/theme.test.ts`: Theme resolution, invalid-value, storage-failure, DOM application, and persistence tests.
- `frontend/src/lib/components/ThemeToggle.svelte`: Accessible Svelte control that delegates theme changes to `theme.ts`.
- `frontend/src/lib/components/ThemeToggle.test.ts`: Toggle accessible-name, document update, and persistence tests.
- `frontend/src/lib/components/BrandMark.svelte`: Shared presentational bank mark used by the header and public layout.
- `frontend/src/lib/pages/AuthLayout.test.ts`: Public-layout semantic structure and responsive composition test.

### Modified Files

- `frontend/package.json`, `frontend/bun.lock`: Add the pinned daisyUI 5 dependency.
- `frontend/src/app.css`: Register daisyUI, define both custom themes, retain global accessibility rules, and remove legacy semantic color tokens after migration.
- `frontend/src/main.ts`: Initialize theme before mounting the SPA.
- `frontend/src/App.svelte`: Restyle app loading, skip link, shell, and main container with daisyUI semantic colors.
- `frontend/src/lib/components/{Button,TextField,Alert}.svelte`: Convert behavioral wrappers to daisyUI classes.
- `frontend/src/lib/components/{Button,TextField,Alert}.test.ts`: Verify daisyUI contracts while preserving semantics.
- `frontend/src/lib/components/{AppHeader,AppFooter,AccountCard}.svelte`: Redesign shell and account presentation.
- `frontend/src/lib/components/{AppHeader,AccountCard}.test.ts`: Preserve behavior and update stable component contracts.
- `frontend/src/lib/pages/{AuthLayout,LoginPage,RegisterPage,VerifyEmailPage}.svelte`: Redesign public experience.
- `frontend/src/lib/pages/{DashboardPage,TransferPage,NewAccountPage,AccountHistoryPage,NotFoundPage}.svelte`: Redesign authenticated pages and state presentation.
- `frontend/src/lib/pages/{TransferPage,NewAccountPage,AccountHistoryPage}.test.ts`: Preserve transactional and stale-response invariants while asserting the new semantic structure.
- `frontend/e2e/accessibility.spec.ts`: Cover both themes, theme persistence, mobile keyboard navigation, responsive overflow, axe, and redesigned screenshots.
- `frontend/README.md`: Document daisyUI and the two-theme behavior.

---

### Task 1: daisyUI And Theme Foundation

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/bun.lock`
- Modify: `frontend/src/app.css`
- Create: `frontend/src/lib/theme.ts`
- Create: `frontend/src/lib/theme.test.ts`
- Modify: `frontend/src/main.ts`

**Interfaces:**
- Produces: `type ThemeName = "simplebank-light" | "simplebank-dark"`
- Produces: `LIGHT_THEME`, `DARK_THEME`, and `THEME_STORAGE_KEY` constants.
- Produces: `resolveTheme(storage?: Pick<Storage, "getItem">, prefersDark?: boolean): ThemeName`.
- Produces: `applyTheme(theme: ThemeName, root?: HTMLElement): ThemeName`.
- Produces: `saveTheme(theme: ThemeName, storage?: Pick<Storage, "setItem">): ThemeName`.
- Produces: `initializeTheme(storage?: Pick<Storage, "getItem">, prefersDark?: boolean, root?: HTMLElement): ThemeName` and `toggleTheme(current: ThemeName): ThemeName`.
- Consumes: Browser `localStorage`, `matchMedia`, and `document.documentElement` only through the functions above.

- [ ] **Step 1: Add failing theme-resolution tests**

Create `frontend/src/lib/theme.test.ts` with concrete cases for saved values,
system fallback, invalid values, storage errors, DOM application, and persistence:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DARK_THEME,
  LIGHT_THEME,
  THEME_STORAGE_KEY,
  applyTheme,
  initializeTheme,
  resolveTheme,
  saveTheme,
  toggleTheme,
} from "./theme";

describe("theme", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-theme");
    vi.restoreAllMocks();
  });

  it("uses a valid saved theme before the system preference", () => {
    const storage = { getItem: vi.fn(() => LIGHT_THEME) };
    expect(resolveTheme(storage, true)).toBe(LIGHT_THEME);
    expect(storage.getItem).toHaveBeenCalledWith(THEME_STORAGE_KEY);
  });

  it("falls back to system preference for missing or invalid storage", () => {
    expect(resolveTheme({ getItem: () => null }, false)).toBe(LIGHT_THEME);
    expect(resolveTheme({ getItem: () => "unknown" }, true)).toBe(DARK_THEME);
  });

  it("falls back when storage access throws", () => {
    const storage = { getItem: () => { throw new DOMException("blocked"); } };
    expect(resolveTheme(storage, true)).toBe(DARK_THEME);
  });

  it("applies, persists, and toggles supported themes", () => {
    const storage = { setItem: vi.fn() };
    expect(applyTheme(DARK_THEME)).toBe(DARK_THEME);
    expect(document.documentElement).toHaveAttribute("data-theme", DARK_THEME);
    expect(saveTheme(LIGHT_THEME, storage)).toBe(LIGHT_THEME);
    expect(storage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, LIGHT_THEME);
    expect(toggleTheme(LIGHT_THEME)).toBe(DARK_THEME);
  });

  it("initializes the resolved theme on the requested root", () => {
    const root = document.createElement("div");
    expect(initializeTheme({ getItem: () => DARK_THEME }, false, root)).toBe(DARK_THEME);
    expect(root).toHaveAttribute("data-theme", DARK_THEME);
  });

  it("does not fail when persistence is unavailable", () => {
    const storage = { setItem: () => { throw new DOMException("blocked"); } };
    expect(saveTheme(DARK_THEME, storage)).toBe(DARK_THEME);
  });
});
```

- [ ] **Step 2: Run the focused test and confirm the missing module failure**

Run: `mise run frontend:test -- src/lib/theme.test.ts`

Expected: FAIL because `./theme` does not exist.

- [ ] **Step 3: Implement the framework-independent theme module**

Create `frontend/src/lib/theme.ts`:

```ts
export const LIGHT_THEME = "simplebank-light";
export const DARK_THEME = "simplebank-dark";
export const THEME_STORAGE_KEY = "simplebank-theme";

export type ThemeName = typeof LIGHT_THEME | typeof DARK_THEME;

function isThemeName(value: string | null): value is ThemeName {
  return value === LIGHT_THEME || value === DARK_THEME;
}

export function resolveTheme(
  storage?: Pick<Storage, "getItem">,
  prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches,
): ThemeName {
  try {
    const saved = (storage ?? window.localStorage).getItem(THEME_STORAGE_KEY);
    if (isThemeName(saved)) return saved;
  } catch {
    // Storage can be unavailable in privacy-restricted contexts.
  }
  return prefersDark ? DARK_THEME : LIGHT_THEME;
}

export function applyTheme(
  theme: ThemeName,
  root: HTMLElement = document.documentElement,
): ThemeName {
  root.dataset.theme = theme;
  return theme;
}

export function saveTheme(
  theme: ThemeName,
  storage?: Pick<Storage, "setItem">,
): ThemeName {
  try {
    (storage ?? window.localStorage).setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // The applied theme remains usable when persistence is unavailable.
  }
  return theme;
}

export function initializeTheme(
  storage?: Pick<Storage, "getItem">,
  prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches,
  root: HTMLElement = document.documentElement,
): ThemeName {
  return applyTheme(resolveTheme(storage, prefersDark), root);
}

export function toggleTheme(current: ThemeName): ThemeName {
  return current === LIGHT_THEME ? DARK_THEME : LIGHT_THEME;
}
```

- [ ] **Step 4: Run theme tests and confirm they pass**

Run: `mise run frontend:test -- src/lib/theme.test.ts`

Expected: PASS.

- [ ] **Step 5: Install daisyUI with the pinned Bun workflow**

Run from `frontend/`: `mise exec -- bun add -D daisyui@^5.7.20`

Expected: `frontend/package.json` gains `daisyui` under `devDependencies` and
`frontend/bun.lock` is updated.

- [ ] **Step 6: Register daisyUI and define both complete themes**

At the top of `frontend/src/app.css`, replace the legacy token block with the
daisyUI plugin and two custom theme declarations. Use this complete semantic
shape, adjusting individual OKLCH values only when browser axe/contrast proof in
Task 8 requires it:

```css
@import "tailwindcss";
@plugin "daisyui" {
  themes: false;
}

@plugin "daisyui/theme" {
  name: "simplebank-light";
  default: true;
  prefersdark: false;
  color-scheme: light;
  --color-base-100: oklch(98% 0.008 190);
  --color-base-200: oklch(95% 0.014 190);
  --color-base-300: oklch(90% 0.02 190);
  --color-base-content: oklch(19% 0.025 205);
  --color-primary: oklch(43% 0.105 190);
  --color-primary-content: oklch(98% 0.01 190);
  --color-secondary: oklch(55% 0.09 225);
  --color-secondary-content: oklch(98% 0.01 225);
  --color-accent: oklch(66% 0.13 160);
  --color-accent-content: oklch(18% 0.03 160);
  --color-neutral: oklch(25% 0.025 205);
  --color-neutral-content: oklch(96% 0.01 190);
  --color-info: oklch(58% 0.13 240);
  --color-info-content: oklch(98% 0.01 240);
  --color-success: oklch(50% 0.13 150);
  --color-success-content: oklch(98% 0.01 150);
  --color-warning: oklch(73% 0.14 80);
  --color-warning-content: oklch(22% 0.04 80);
  --color-error: oklch(54% 0.19 28);
  --color-error-content: oklch(98% 0.01 28);
  --radius-selector: 0.5rem;
  --radius-field: 0.375rem;
  --radius-box: 0.75rem;
  --size-selector: 0.25rem;
  --size-field: 0.25rem;
  --border: 1px;
  --depth: 1;
  --noise: 0;
}

@plugin "daisyui/theme" {
  name: "simplebank-dark";
  default: false;
  prefersdark: true;
  color-scheme: dark;
  --color-base-100: oklch(20% 0.025 205);
  --color-base-200: oklch(17% 0.024 205);
  --color-base-300: oklch(27% 0.03 205);
  --color-base-content: oklch(94% 0.012 190);
  --color-primary: oklch(72% 0.12 185);
  --color-primary-content: oklch(18% 0.035 190);
  --color-secondary: oklch(70% 0.1 225);
  --color-secondary-content: oklch(18% 0.03 225);
  --color-accent: oklch(75% 0.13 155);
  --color-accent-content: oklch(18% 0.03 155);
  --color-neutral: oklch(33% 0.035 205);
  --color-neutral-content: oklch(95% 0.01 190);
  --color-info: oklch(72% 0.12 235);
  --color-info-content: oklch(18% 0.03 235);
  --color-success: oklch(70% 0.13 150);
  --color-success-content: oklch(18% 0.03 150);
  --color-warning: oklch(79% 0.13 80);
  --color-warning-content: oklch(20% 0.04 80);
  --color-error: oklch(72% 0.16 28);
  --color-error-content: oklch(18% 0.04 28);
  --radius-selector: 0.5rem;
  --radius-field: 0.375rem;
  --radius-box: 0.75rem;
  --size-selector: 0.25rem;
  --size-field: 0.25rem;
  --border: 1px;
  --depth: 1;
  --noise: 0;
}

@theme {
  --font-sans: "IBM Plex Sans Variable", ui-sans-serif, sans-serif;
  --font-mono: "Liberation Mono", monospace;
}
```

Keep the existing global height, font rendering, focus-visible, programmatic
focus, and reduced-motion rules. Change body colors to
`background-color: var(--color-base-200)` and
`color: var(--color-base-content)`. Do not delete legacy color tokens until all
markup is migrated in Task 7, so intermediate commits remain buildable.

- [ ] **Step 7: Initialize the theme before Svelte mount**

Update `frontend/src/main.ts` so initialization occurs before `mount`:

```ts
import { mount } from "svelte";
import "@fontsource-variable/ibm-plex-sans/wght.css";
import "./app.css";
import App from "./App.svelte";
import { initializeTheme } from "./lib/theme";

initializeTheme();

const app = mount(App, {
  target: document.getElementById("app")!,
});

export default app;
```

- [ ] **Step 8: Verify theme foundation and production CSS compilation**

Run:

```sh
mise run frontend:test -- src/lib/theme.test.ts
mise run frontend:check
mise run frontend:build
```

Expected: all commands PASS and the production CSS contains daisyUI output.

- [ ] **Step 9: Commit the foundation**

```sh
git add frontend/package.json frontend/bun.lock frontend/src/app.css frontend/src/main.ts frontend/src/lib/theme.ts frontend/src/lib/theme.test.ts
git commit -m "feat(frontend): add daisyui theme foundation"
```

---

### Task 2: Shared daisyUI Primitives And Theme Toggle

**Files:**
- Create: `frontend/src/lib/components/BrandMark.svelte`
- Create: `frontend/src/lib/components/ThemeToggle.svelte`
- Create: `frontend/src/lib/components/ThemeToggle.test.ts`
- Modify: `frontend/src/lib/components/Button.svelte`
- Modify: `frontend/src/lib/components/Button.test.ts`
- Modify: `frontend/src/lib/components/TextField.svelte`
- Modify: `frontend/src/lib/components/TextField.test.ts`
- Modify: `frontend/src/lib/components/Alert.svelte`
- Modify: `frontend/src/lib/components/Alert.test.ts`

**Interfaces:**
- Consumes: Theme constants/functions from Task 1.
- Produces: `BrandMark` props `{ compact?: boolean }` with decorative icon and visible `SimpleBank` text.
- Produces: `ThemeToggle` with no props; its button name is `Switch to dark theme` or `Switch to light theme`.
- Preserves: Existing `Button`, `TextField`, and `Alert` public prop interfaces exactly.

- [ ] **Step 1: Add failing daisyUI primitive and theme-toggle tests**

Extend the existing component tests with stable semantic class contracts:

```ts
// Button.test.ts
expect(button).toHaveClass("btn", "btn-primary");
expect(button.querySelector(".loading-spinner")).toBeInTheDocument();

// TextField.test.ts
expect(field).toHaveClass("input", "w-full");
expect(field.closest("fieldset")).toHaveClass("fieldset");

// Alert.test.ts, inside each variant case
expect(alert).toHaveClass("alert");
expect(alert).toHaveClass(
  variant === "error" ? "alert-error" : variant === "success" ? "alert-success" : "alert-info",
);
```

Create `ThemeToggle.test.ts`:

```ts
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import { DARK_THEME, LIGHT_THEME, THEME_STORAGE_KEY } from "../theme";
import ThemeToggle from "./ThemeToggle.svelte";

describe("ThemeToggle", () => {
  afterEach(() => {
    cleanup();
    localStorage.clear();
    document.documentElement.dataset.theme = LIGHT_THEME;
  });

  it("describes and persists the theme it will switch to", async () => {
    document.documentElement.dataset.theme = LIGHT_THEME;
    render(ThemeToggle);

    await fireEvent.click(screen.getByRole("button", { name: "Switch to dark theme" }));

    expect(document.documentElement).toHaveAttribute("data-theme", DARK_THEME);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe(DARK_THEME);
    expect(screen.getByRole("button", { name: "Switch to light theme" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run focused component tests and confirm class/toggle failures**

Run:

```sh
mise run frontend:test -- src/lib/components/Button.test.ts src/lib/components/TextField.test.ts src/lib/components/Alert.test.ts src/lib/components/ThemeToggle.test.ts
```

Expected: FAIL because the shared controls lack daisyUI classes and the toggle
does not exist.

- [ ] **Step 3: Convert `Button.svelte` to daisyUI without changing behavior**

Replace its class maps and loading icon with:

```svelte
<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    type?: "button" | "submit";
    variant?: "primary" | "secondary" | "ghost";
    disabled?: boolean;
    loading?: boolean;
    class?: string;
    onclick?: (event: MouseEvent) => void;
    children: Snippet;
  }

  let { type = "button", variant = "primary", disabled = false, loading = false,
    class: className = "", onclick, children }: Props = $props();

  const variants = {
    primary: "btn-primary",
    secondary: "btn-outline",
    ghost: "btn-ghost",
  };
</script>

<button
  {type}
  class={`btn min-h-11 ${variants[variant]} ${className}`}
  disabled={disabled || loading}
  aria-busy={loading}
  {onclick}
>
  {#if loading}<span class="loading loading-spinner loading-sm" aria-hidden="true"></span>{/if}
  {@render children()}
</button>
```

- [ ] **Step 4: Convert `TextField.svelte` to daisyUI while preserving associations**

Keep the script unchanged and replace only markup/classes with this structure:

```svelte
<fieldset class="fieldset">
  <label for={id} class="fieldset-legend contrast-more:font-bold">{label}</label>
  <input
    {id} {type} {placeholder} {autocomplete} {required} {inputmode} {step} {min} {max}
    {disabled} {oninput} bind:value bind:this={inputElement}
    aria-describedby={describedBy}
    aria-invalid={error ? true : undefined}
    class="input w-full min-h-11 disabled:cursor-not-allowed contrast-more:border-2 forced-colors:border-[ButtonBorder] {error
      ? 'input-error forced-colors:aria-invalid:border-[Mark]'
      : ''}"
  />
  {#if error}
    <p id={errorId} role="alert" class="label text-error contrast-more:font-bold">{error}</p>
  {/if}
  {#if hint}<p id={hintId} class="label text-base-content/70">{hint}</p>{/if}
</fieldset>
```

- [ ] **Step 5: Convert `Alert.svelte` to semantic daisyUI variants**

Keep role derivation and replace style mapping/markup with:

```svelte
<script lang="ts">
  import type { Snippet } from "svelte";
  interface Props { variant?: "error" | "success" | "info"; children: Snippet; }
  let { variant = "info", children }: Props = $props();
  const styles = { error: "alert-error", success: "alert-success", info: "alert-info" };
  const role = $derived(variant === "error" ? "alert" : "status");
</script>

<div
  {role}
  class={`alert ${styles[variant]} text-sm forced-colors:border-[CanvasText] forced-colors:bg-[Canvas] forced-colors:text-[CanvasText]`}
>
  <div>{@render children()}</div>
</div>
```

- [ ] **Step 6: Add the shared brand mark**

Create `BrandMark.svelte` with a decorative bank icon, visible name, and no link
behavior so callers retain navigation semantics:

```svelte
<script lang="ts">
  interface Props { compact?: boolean; }
  let { compact = false }: Props = $props();
</script>

<span class="inline-flex items-center gap-2.5 font-semibold tracking-tight">
  <span class="grid size-9 place-items-center rounded-box bg-primary text-primary-content" aria-hidden="true">
    <svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M3 10 12 4l9 6" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M5 10v8h14v-8" stroke-linecap="round" stroke-linejoin="round" />
      <path d="M9 18v-5h6v5" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
  </span>
  <span class:sr-only={compact}>SimpleBank</span>
</span>
```

- [ ] **Step 7: Implement the accessible theme toggle**

Create `ThemeToggle.svelte`:

```svelte
<script lang="ts">
  import Moon from "@lucide/svelte/icons/moon";
  import Sun from "@lucide/svelte/icons/sun";
  import { DARK_THEME, LIGHT_THEME, applyTheme, saveTheme, toggleTheme, type ThemeName } from "../theme";

  let current = $state<ThemeName>(
    document.documentElement.dataset.theme === DARK_THEME ? DARK_THEME : LIGHT_THEME,
  );
  const nextLabel = $derived(current === LIGHT_THEME ? "dark" : "light");

  function changeTheme() {
    current = toggleTheme(current);
    applyTheme(current);
    saveTheme(current);
  }
</script>

<button
  type="button"
  class="btn btn-ghost btn-square min-h-11 min-w-11"
  aria-label={`Switch to ${nextLabel} theme`}
  onclick={changeTheme}
>
  {#if current === LIGHT_THEME}<Moon aria-hidden="true" size={19} />{:else}<Sun aria-hidden="true" size={19} />{/if}
</button>
```

- [ ] **Step 8: Run focused tests and all frontend unit tests**

Run:

```sh
mise run frontend:test -- src/lib/components/Button.test.ts src/lib/components/TextField.test.ts src/lib/components/Alert.test.ts src/lib/components/ThemeToggle.test.ts
mise run frontend:test
```

Expected: PASS. If tests query the loading icon by SVG, update them to query
`.loading-spinner`; do not weaken busy/disabled assertions.

- [ ] **Step 9: Commit the shared component migration**

```sh
git add frontend/src/lib/components/BrandMark.svelte frontend/src/lib/components/ThemeToggle.svelte frontend/src/lib/components/ThemeToggle.test.ts frontend/src/lib/components/Button.svelte frontend/src/lib/components/Button.test.ts frontend/src/lib/components/TextField.svelte frontend/src/lib/components/TextField.test.ts frontend/src/lib/components/Alert.svelte frontend/src/lib/components/Alert.test.ts
git commit -m "feat(frontend): migrate shared controls to daisyui"
```

---

### Task 3: Responsive Application Shell

**Files:**
- Modify: `frontend/src/App.svelte`
- Modify: `frontend/src/lib/components/AppHeader.svelte`
- Modify: `frontend/src/lib/components/AppHeader.test.ts`
- Modify: `frontend/src/lib/components/AppFooter.svelte`

**Interfaces:**
- Consumes: `BrandMark`, `ThemeToggle`, existing `Link`, auth/account stores, and router.
- Preserves: Mobile menu `aria-expanded`, Escape close, route-change close, and focus restoration.
- Produces: Header theme control and a daisyUI `navbar`/`menu` structure.

- [ ] **Step 1: Add failing shell tests for the new contracts**

In `AppHeader.test.ts`, replace brittle legacy marker assertions with:

```ts
it("uses daisyUI navigation and exposes theme switching", () => {
  auth.user = user;
  auth.accessToken = "access-token";
  render(AppHeader);

  expect(screen.getByRole("banner").querySelector(".navbar")).toBeInTheDocument();
  expect(screen.getByRole("navigation", { name: "Primary" }).querySelector(".menu")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /switch to (dark|light) theme/i })).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute("aria-current", "page");
});
```

Retain tests for mobile disclosure, focus restoration, long identity, failed
logout cleanup, and hidden menu icons. Remove only assertions tied to the old
`border-l-2` active marker.

- [ ] **Step 2: Run header tests and confirm the navbar/theme failures**

Run: `mise run frontend:test -- src/lib/components/AppHeader.test.ts`

Expected: FAIL because the header lacks daisyUI shell classes and theme control.

- [ ] **Step 3: Redesign `AppHeader.svelte` with navbar and menu components**

Preserve the complete script behavior. Replace duplicated branding with
`BrandMark`, add `ThemeToggle`, and use this semantic shape:

```svelte
<header class="border-b border-base-300 bg-base-100">
  <div class="navbar mx-auto min-h-16 max-w-7xl gap-2 px-4 sm:px-6">
    <div class="navbar-start min-w-0 gap-2">
      <button
        bind:this={menuButton}
        type="button"
        class="btn btn-ghost btn-square min-h-11 min-w-11 sm:hidden"
        aria-label={menuOpen ? "Close navigation" : "Open navigation"}
        aria-controls="mobile-primary-navigation"
        aria-expanded={menuOpen}
        onclick={toggleMenu}
      >
        {#if menuOpen}<X aria-hidden="true" size={20} />{:else}<Menu aria-hidden="true" size={20} />{/if}
      </button>
      <Link href="/" class="btn btn-ghost h-auto min-h-11 px-1 text-lg"><BrandMark /></Link>
    </div>
    <nav aria-label="Primary" class="navbar-center hidden sm:flex">
      <ul class="menu menu-horizontal gap-1 p-0">
        {#each nav as item (item.href)}
          <li>
            <Link href={item.href} class="rounded-field aria-[current=page]:bg-primary aria-[current=page]:text-primary-content">{item.label}</Link>
          </li>
        {/each}
      </ul>
    </nav>
    <div class="navbar-end min-w-0 gap-1 sm:gap-2">
      <span class="hidden min-w-0 max-w-48 truncate text-sm font-medium md:block" title={auth.user?.email}>{userName}</span>
      <ThemeToggle />
      <button type="button" class="btn btn-ghost min-h-11 whitespace-nowrap" onclick={logout} disabled={signingOut}>
        <LogOut aria-hidden="true" size={16} />
        {signingOut ? "Signing out…" : "Sign out"}
      </button>
    </div>
  </div>
  {#if logoutError}<p role="alert" class="mx-auto max-w-7xl px-4 pb-3 text-sm text-error sm:px-6">{logoutError}</p>{/if}
  {#if menuOpen}
    <nav id="mobile-primary-navigation" aria-label="Mobile primary" class="border-t border-base-300 px-4 py-3 sm:hidden">
      <ul class="menu w-full gap-1 p-0">
        {#each nav as item (item.href)}
          <li><Link href={item.href} class="aria-[current=page]:bg-primary aria-[current=page]:text-primary-content" onclick={closeMenuAndRestoreFocus}>{item.label}</Link></li>
        {/each}
      </ul>
    </nav>
  {/if}
</header>
```

Keep the mobile button before branding at narrow widths, keep its existing
accessible names, and retain `closeMenuAndRestoreFocus` for link selection.

- [ ] **Step 4: Redesign `App.svelte` shell without changing routing effects**

Leave lines implementing auth/reset, view resolution, title/announcement, focus,
and redirects logically unchanged. Convert presentation to:

```svelte
{#if auth.initializing}
  <div class="grid min-h-screen place-items-center bg-base-200" aria-busy="true">
    <span class="loading loading-ring loading-lg text-primary" aria-label="Loading"></span>
  </div>
{:else}
  <div class="flex min-h-screen flex-col bg-base-200 text-base-content">
    {#if view.chrome}
      <a href="#main" class="btn btn-primary sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50">Skip to content</a>
      <AppHeader />
      <main id="main" class="mx-auto w-full max-w-7xl flex-1 px-4 py-8 sm:px-6 lg:py-12"><Page /></main>
    {:else}
      <Page />
    {/if}
    <AppFooter />
  </div>
{/if}
```

- [ ] **Step 5: Redesign footer and update its stack list**

Use daisyUI footer/base tokens and add daisyUI after Tailwind CSS:

```ts
{ name: "Tailwind CSS", href: "https://tailwindcss.com" },
{ name: "daisyUI", href: "https://daisyui.com" },
```

Use `footer footer-vertical border-t border-base-300 bg-base-100 px-4 py-6 text-sm text-base-content/65 sm:footer-horizontal sm:px-6`, semantic links with `link link-hover`, and preserve version output and external-link security attributes.

- [ ] **Step 6: Run shell and route regression tests**

Run:

```sh
mise run frontend:test -- src/lib/components/AppHeader.test.ts src/App.test.ts
mise run frontend:check
```

Expected: PASS, including route focus and failed-logout cleanup.

- [ ] **Step 7: Commit the application shell**

```sh
git add frontend/src/App.svelte frontend/src/lib/components/AppHeader.svelte frontend/src/lib/components/AppHeader.test.ts frontend/src/lib/components/AppFooter.svelte
git commit -m "feat(frontend): redesign responsive application shell"
```

---

### Task 4: Public Authentication And Verification Experience

**Files:**
- Modify: `frontend/src/lib/pages/AuthLayout.svelte`
- Create: `frontend/src/lib/pages/AuthLayout.test.ts`
- Modify: `frontend/src/lib/pages/LoginPage.svelte`
- Modify: `frontend/src/lib/pages/RegisterPage.svelte`
- Modify: `frontend/src/lib/pages/VerifyEmailPage.svelte`

**Interfaces:**
- Consumes: `BrandMark`, converted `Button`, `TextField`, `Alert`, and existing `Link`.
- Preserves: `AuthLayout` props `{ title, subtitle, children, footer }`.
- Preserves: Login/register request behavior, one-shot registration notice, verification query capture/removal, and auth-dependent destination.

- [ ] **Step 1: Add a failing semantic test for the editorial auth layout**

Create `frontend/src/lib/pages/AuthLayout.test.ts`:

```ts
import { createRawSnippet } from "svelte";
import { render, screen } from "@testing-library/svelte";
import { describe, expect, it } from "vitest";
import AuthLayout from "./AuthLayout.svelte";

describe("AuthLayout", () => {
  it("pairs an editorial introduction with a daisyUI task card", () => {
    const children = createRawSnippet(() => ({ render: () => "Form content" }));
    const footer = createRawSnippet(() => ({ render: () => "Footer content" }));
    const { container } = render(AuthLayout, {
      title: "Welcome back",
      subtitle: "Sign in to your account.",
      children,
      footer,
    });

    expect(screen.getByRole("region", { name: "SimpleBank introduction" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Welcome back" })).toBeInTheDocument();
    expect(container.querySelector(".card.card-border")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the auth-layout test and confirm the daisyUI structure failure**

Run:

```sh
mise run frontend:test -- src/lib/pages/AuthLayout.test.ts
```

Expected: FAIL because the layout lacks the named editorial section and
`card-border` structure.

- [ ] **Step 3: Redesign `AuthLayout.svelte` as a responsive editorial split**

Replace duplicate logo and card markup with this responsive composition:

```svelte
<main class="grid min-h-[calc(100vh-5rem)] flex-1 lg:grid-cols-[minmax(0,1fr)_minmax(28rem,0.8fr)]">
  <section class="hidden bg-neutral px-10 py-14 text-neutral-content lg:flex lg:flex-col lg:justify-between" aria-label="SimpleBank introduction">
    <BrandMark />
    <div class="max-w-xl">
      <p class="text-sm font-semibold tracking-[0.18em] text-primary uppercase">Clear money movement</p>
      <p class="mt-5 text-5xl leading-[1.05] font-semibold tracking-tight">Banking built around what matters.</p>
      <p class="mt-5 max-w-md text-base text-neutral-content/70">See every balance clearly, move money confidently, and stay in control.</p>
    </div>
    <p class="text-sm text-neutral-content/60">Secure by design. Simple by default.</p>
  </section>
  <section class="flex items-center justify-center px-4 py-10 sm:px-8">
    <div class="w-full max-w-md">
      <div class="mb-8 lg:hidden"><BrandMark /></div>
      <div class="card card-border bg-base-100 shadow-sm">
        <div class="card-body gap-0 p-6 sm:p-8">
          <h1 class="card-title text-3xl tracking-tight">{title}</h1>
          <p class="mt-2 text-base-content/65">{subtitle}</p>
          <div class="mt-7">{@render children()}</div>
        </div>
      </div>
      <p class="mt-6 text-center text-sm text-base-content/65">{@render footer()}</p>
    </div>
  </section>
</main>
```

- [ ] **Step 4: Convert login/register call-site classes to daisyUI semantics**

Keep both scripts and form behavior unchanged. Use `gap-5`, full-width primary
buttons, and `link link-primary font-semibold` for auth links:

```svelte
<form class="flex flex-col gap-5" onsubmit={handleSubmit} novalidate>
  <!-- existing alerts and fields -->
  <Button type="submit" loading={submitting} class="mt-2 w-full">Sign in</Button>
</form>
```

Apply the equivalent structure to registration. Do not change validation copy,
autocomplete attributes, submission handlers, or redirect state.

- [ ] **Step 5: Redesign verification states with daisyUI status vocabulary**

Keep the script unchanged. Use `loading loading-ring loading-lg text-primary`
for pending, `bg-success/15 text-success` for success icon treatment,
`bg-error/15 text-error` for failure, and `btn btn-primary`/`btn btn-outline`
for destination links. Preserve exactly one `role="status"` in pending/success
states and one `role="alert"` in failure state.

- [ ] **Step 6: Run auth, route, and rate-limit regressions**

Run:

```sh
mise run frontend:test
mise run frontend:test:e2e -- e2e/rate-limit.spec.ts
mise run frontend:check
```

Expected: PASS; the 429 response still produces the exact retry message and the
button is re-enabled.

- [ ] **Step 7: Commit public-page redesign**

```sh
git add frontend/src/lib/pages/AuthLayout.svelte frontend/src/lib/pages/AuthLayout.test.ts frontend/src/lib/pages/LoginPage.svelte frontend/src/lib/pages/RegisterPage.svelte frontend/src/lib/pages/VerifyEmailPage.svelte
git commit -m "feat(frontend): redesign public authentication pages"
```

---

### Task 5: Dashboard And Account Cards

**Files:**
- Modify: `frontend/src/lib/pages/DashboardPage.svelte`
- Modify: `frontend/src/lib/components/AccountCard.svelte`
- Modify: `frontend/src/lib/components/AccountCard.test.ts`

**Interfaces:**
- Consumes: Existing account store, `formatMoney`, `Alert`, `Link`, and daisyUI `stats`, `card`, `badge`, `skeleton`, and button classes.
- Preserves: Account loading trigger, per-currency total calculation, copy timer cleanup, transfer source selection, and activity links.
- Produces: Currency summaries as a semantic `dl` containing daisyUI stat entries.

- [ ] **Step 1: Add failing semantic structure tests for the redesigned account card**

Extend `AccountCard.test.ts`:

```ts
it("uses a daisyUI card with an accessible balance label", () => {
  const { container } = render(AccountCard, { props: { account } });
  expect(container.querySelector("article.card.card-border")).toBeInTheDocument();
  expect(screen.getByText("Available balance")).toBeInTheDocument();
  expect(screen.getByText("USD")).toHaveClass("badge", "badge-outline");
});
```

Keep every existing clipboard, timer, transfer-action, and activity-link test.

- [ ] **Step 2: Run account-card tests and confirm the semantic failures**

Run: `mise run frontend:test -- src/lib/components/AccountCard.test.ts`

Expected: FAIL because the existing article is not a daisyUI card and lacks the
balance label.

- [ ] **Step 3: Redesign `AccountCard.svelte` around balance hierarchy**

Keep the script unchanged. Use this content order and daisyUI vocabulary:

```svelte
<article class="card card-border bg-base-100 shadow-sm transition-transform hover:-translate-y-0.5 motion-reduce:transform-none">
  <div class="card-body gap-5 p-5 sm:p-6">
    <div class="flex items-center justify-between gap-3">
      <span class="badge badge-outline font-semibold">{account.currency}</span>
      <span class="text-xs text-base-content/55">Opened {created}</span>
    </div>
    <div>
      <p class="text-xs font-medium tracking-wide text-base-content/55 uppercase">Available balance</p>
      <p class="mt-1 text-3xl font-semibold tracking-tight tabular-nums">{formatMoney(account.balance, account.currency)}</p>
    </div>
    <div class="rounded-box bg-base-200 p-3">
      <p class="text-xs font-medium text-base-content/55">Account number</p>
      <code class="mt-1 block font-mono text-xs break-all">{account.id}</code>
      <button type="button" onclick={copyId} class="btn btn-ghost btn-sm mt-2 min-h-11" aria-label={copied ? "Account number copied" : "Copy account number"}>
        {#if copied}<Check size={14} aria-hidden="true" />{:else}<Copy size={14} aria-hidden="true" />{/if}
        {copied ? "Copied" : "Copy"}
      </button>
    </div>
    <div class="card-actions grid grid-cols-2">
      <button type="button" class="btn min-h-11" onclick={sendFromHere}><Send size={16} aria-hidden="true" />Send money</button>
      <Link href={`/accounts/${account.id}`} class="btn btn-ghost min-h-11"><History size={16} aria-hidden="true" />Activity</Link>
    </div>
  </div>
</article>
```

- [ ] **Step 4: Redesign dashboard hero, stats, state panels, and card grid**

Keep the script and totals derivation unchanged. Implement:

- A dark neutral hero card with `badge`, editorial greeting, account count, and
  `btn btn-primary` / `btn btn-outline` actions.
- A `dl.stats stats-vertical lg:stats-horizontal` block nested in the hero, with
  one `.stat` per currency and `.stat-value` using tabular numerals.
- `skeleton h-52` loading cards under the existing busy label.
- A `card border border-dashed border-base-300 bg-base-100` empty state with one
  primary account-opening action.
- The account grid remains one/two/three columns at current breakpoints.

Use `text-base-content/60` for secondary text, not legacy `text-muted`; use
semantic daisyUI colors for all states.

- [ ] **Step 5: Run account and store regression tests**

Run:

```sh
mise run frontend:test -- src/lib/components/AccountCard.test.ts src/lib/stores/accounts.svelte.test.ts
mise run frontend:check
```

Expected: PASS, including clipboard timer cleanup and session generation tests.

- [ ] **Step 6: Commit dashboard redesign**

```sh
git add frontend/src/lib/pages/DashboardPage.svelte frontend/src/lib/components/AccountCard.svelte frontend/src/lib/components/AccountCard.test.ts
git commit -m "feat(frontend): redesign dashboard and account cards"
```

---

### Task 6: Transfer And Account-Opening Forms

**Files:**
- Modify: `frontend/src/lib/pages/TransferPage.svelte`
- Modify: `frontend/src/lib/pages/TransferPage.test.ts`
- Modify: `frontend/src/lib/pages/NewAccountPage.svelte`
- Modify: `frontend/src/lib/pages/NewAccountPage.test.ts`

**Interfaces:**
- Consumes: Converted shared controls and native `select`/`radio` elements with daisyUI classes.
- Preserves: Stable transfer idempotency key across failures, rotation after success, account refresh, limit validation, policy-load concurrency, disabled policy state, currency availability, and opening-deposit validation.
- Produces: Transfer receipt remains one `role="status"` with a definition list.

- [ ] **Step 1: Add failing form presentation contracts without weakening invariant tests**

Add to `TransferPage.test.ts`:

```ts
it("presents the source account as a daisyUI select", async () => {
  fetchMock.mockResolvedValueOnce(jsonResponse(200, {}));
  render(TransferPage);
  const source = screen.getByRole("combobox", { name: "From account" });
  expect(source).toHaveClass("select", "w-full");
});
```

In the existing receipt test, add:

```ts
expect(receipt).toHaveClass("card");
```

Add to `NewAccountPage.test.ts` after policy load:

```ts
it("uses native daisyUI radios for currency choices", async () => {
  render(NewAccountPage);
  const euro = await screen.findByRole("radio", { name: /EUR/ });
  expect(euro).toHaveClass("radio", "radio-primary");
});
```

- [ ] **Step 2: Run focused page tests and confirm class failures**

Run:

```sh
mise run frontend:test -- src/lib/pages/TransferPage.test.ts src/lib/pages/NewAccountPage.test.ts
```

Expected: FAIL only on new daisyUI structure assertions.

- [ ] **Step 3: Redesign transfer page while leaving its script unchanged**

Use a shared page pattern: `max-w-2xl`, back link with `btn btn-ghost`, editorial
heading, and one bordered `card bg-base-100`. Inside the card:

```svelte
<form class="card-body gap-5 p-6 sm:p-8" onsubmit={handleSubmit} novalidate>
  {#if error}<Alert variant="error">{error}</Alert>{/if}
  <fieldset class="fieldset">
    <label for="from" class="fieldset-legend">From account</label>
    <select id="from" bind:value={fromAccountId} class="select w-full min-h-11">
      {#each accounts.items as account (account.id)}
        <option value={account.id}>{account.currency} · {formatMoney(account.balance, account.currency)}</option>
      {/each}
    </select>
    {#if fromAccount}<p class="label text-base-content/65">Available: {formatMoney(fromAccount.balance, fromAccount.currency)}</p>{/if}
  </fieldset>
  <!-- existing TextField instances -->
  <Button type="submit" loading={submitting} class="mt-2 w-full sm:w-auto">Send transfer</Button>
</form>
```

Render the receipt as `card card-border border-success/30 bg-success/10`, keep the
single status role on the outer card, and keep amount, remaining balance, and
reference in a `dl`. Do not move or alter `idempotencyKey` initialization or its
rotation line.

- [ ] **Step 4: Redesign new-account page while leaving policy logic unchanged**

Use the same page heading/card pattern. Keep policy alerts above choices. Render
currency choices as labels containing native radios:

```svelte
<fieldset class="fieldset gap-3" aria-busy={policyLoading}>
  <legend class="fieldset-legend">Currency</legend>
  <div class="grid gap-3 sm:grid-cols-3">
    {#each available as code (code)}
      <label class="label min-h-20 cursor-pointer rounded-box border border-base-300 bg-base-100 p-4 has-[:checked]:border-primary has-[:checked]:bg-primary/10">
        <span><span class="block font-semibold">{code}</span><span class="text-xs text-base-content/60">Starts at {formatMoney(0, code)}</span></span>
        <input type="radio" name="currency" value={code} bind:group={currency} disabled={formDisabled} class="radio radio-primary" />
      </label>
    {/each}
  </div>
</fieldset>
```

Keep the `TextField` max/step/hint bindings and disable submit until policy data
is ready.

- [ ] **Step 5: Run transactional tests and verify invariants explicitly**

Run:

```sh
mise run frontend:test -- src/lib/pages/TransferPage.test.ts src/lib/pages/NewAccountPage.test.ts src/lib/opening-limits.test.ts
mise run frontend:check
```

Expected: PASS. Confirm the transfer retry test sends byte-equivalent logical
request bodies with the same idempotency key and the new-account concurrency
test still starts policy loading before account loading resolves.

- [ ] **Step 6: Commit form redesigns**

```sh
git add frontend/src/lib/pages/TransferPage.svelte frontend/src/lib/pages/TransferPage.test.ts frontend/src/lib/pages/NewAccountPage.svelte frontend/src/lib/pages/NewAccountPage.test.ts
git commit -m "feat(frontend): redesign banking forms"
```

---

### Task 7: Account Activity, Not Found, And Legacy Token Removal

**Files:**
- Modify: `frontend/src/lib/pages/AccountHistoryPage.svelte`
- Modify: `frontend/src/lib/pages/AccountHistoryPage.test.ts`
- Modify: `frontend/src/lib/pages/NotFoundPage.svelte`
- Modify: `frontend/src/app.css`
- Test: All `frontend/src/**/*.test.ts`

**Interfaces:**
- Preserves: Route-derived account id, cached/deep-link account loading, stale generation guard, transfer direction, and auth-dependent not-found destination.
- Produces: Activity list uses daisyUI `list`/`list-row` semantics and signed amounts remain unchanged.
- Removes: All legacy `canvas`, `surface`, `surface-raised`, `surface-muted`, `border`, `control`, `ink`, `muted`, `brand`, `brand-strong`, `brand-soft`, `positive`, `negative`, `info-soft`, and `attention` theme utilities.

- [ ] **Step 1: Add failing activity-list structure test**

Extend `AccountHistoryPage.test.ts` with a transfer fixture and this assertion:

```ts
it("renders loaded transfers as a daisyUI activity list", async () => {
  requestMock.mockImplementation((path: string) => Promise.resolve(
    path.includes("/transfers")
      ? [{ id: "tx-1", from_account_id: accountA, to_account_id: accountB, amount: 2500, created_at: "2026-01-02T00:00:00Z" }]
      : account(accountA),
  ));
  const { container } = render(AccountHistoryPage);
  expect(await screen.findByText("Sent")).toBeInTheDocument();
  expect(container.querySelector("ul.list")).toBeInTheDocument();
  expect(container.querySelector("li.list-row")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run account-history tests and confirm list class failure**

Run: `mise run frontend:test -- src/lib/pages/AccountHistoryPage.test.ts`

Expected: FAIL on `.list`/`.list-row`; stale-response tests remain green.

- [ ] **Step 3: Redesign account activity while preserving the complete script**

Use a `max-w-3xl` page, daisyUI back button, and a header card showing currency
badge, account id, and current balance. Convert states to:

- Error: existing `Alert` with `btn btn-link min-h-11` retry.
- Loading: four `skeleton h-20` blocks under existing busy semantics.
- Empty: centered bordered `card` with one `btn btn-primary` action.
- Loaded: `<ul class="list rounded-box border border-base-300 bg-base-100 shadow-sm">`
  with each transfer in `<li class="list-row items-center">`.

Use `text-error` for outgoing, `text-success` for incoming, and preserve the
exact `formatSignedMoney` output and counterparty/date content.

- [ ] **Step 4: Redesign not-found page with daisyUI hero/card vocabulary**

Keep its auth-derived `home` unchanged. Use a compact `hero`, prominent `404`
eyebrow in primary, and `Link` styled `btn btn-primary min-h-11`.

- [ ] **Step 5: Remove all obsolete token definitions and references**

Search source markup:

```sh
rg -n "(^|[-:/])(canvas|surface(-raised|-muted)?|border|control|ink|muted|brand(-strong|-soft)?|positive(-soft)?|negative(-soft)?|info-soft|attention(-soft)?)([ /\"']|$)" frontend/src --glob '*.svelte' --glob '*.css'
```

Expected before cleanup: remaining matches identify legacy color utility usage.
Replace each with daisyUI semantics:

```text
bg-canvas -> bg-base-200
bg-surface / bg-surface-raised -> bg-base-100
bg-surface-muted -> bg-base-200
border-border / border-control -> border-base-300
text-ink -> text-base-content
text-muted -> text-base-content/60 or /70 according to hierarchy
bg-brand / text-brand -> bg-primary / text-primary
text-positive -> text-success
text-negative -> text-error
```

Then delete the corresponding legacy color/radius/shadow variables from the
`@theme` block, leaving only font families. Do not replace native CSS property
names or ordinary prose containing words such as `border`.

- [ ] **Step 6: Run full unit suite and static checks**

Run:

```sh
mise run frontend:test
mise run frontend:check
mise run frontend:lint
```

Expected: PASS. Repeat the targeted legacy utility search and confirm no matches
remain in Svelte class names or CSS token declarations.

- [ ] **Step 7: Commit final page migration and token cleanup**

```sh
git add frontend/src/lib/pages/AccountHistoryPage.svelte frontend/src/lib/pages/AccountHistoryPage.test.ts frontend/src/lib/pages/NotFoundPage.svelte frontend/src/app.css
git commit -m "feat(frontend): complete daisyui page migration"
```

---

### Task 8: Browser Coverage, Documentation, And Release Proof

**Files:**
- Modify: `frontend/e2e/accessibility.spec.ts`
- Modify: `frontend/e2e/accessibility.spec.ts-snapshots/dashboard-320-linux.png`
- Modify: `frontend/e2e/accessibility.spec.ts-snapshots/dashboard-1440-linux.png`
- Modify: `frontend/README.md`

**Interfaces:**
- Consumes: Complete redesigned SPA from Tasks 1-7.
- Produces: Browser proof for both themes, persisted toggling, responsive accessibility, keyboard mobile navigation, and stable visual baselines.

- [ ] **Step 1: Add failing browser tests for theme initialization and persistence**

Add to `frontend/e2e/accessibility.spec.ts`:

```ts
test("theme selection is accessible, persisted, and valid after reload", async ({ page }) => {
  await mockAuthenticatedAPI(page);
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");

  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-light");
  await page.getByRole("button", { name: "Switch to dark theme" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-dark");
  await expect(page.getByRole("button", { name: "Switch to light theme" })).toBeVisible();
  await expectNoAccessibilityViolations(page);
});

test("system dark preference initializes the dark theme without a saved value", async ({ page }) => {
  await mockAuthenticatedAPI(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "simplebank-dark");
});
```

Update the viewport accessibility test to run axe once in each supported theme
at 320 and 1440, while retaining overflow checks at all four current viewports.

Also add a public-page theme matrix so both themes are proven outside the
authenticated shell:

```ts
for (const theme of ["simplebank-light", "simplebank-dark"] as const) {
  test(`login remains accessible in ${theme}`, async ({ page }) => {
    await page.addInitScript(
      ({ key, value }) => localStorage.setItem(key, value),
      { key: "simplebank-theme", value: theme },
    );
    await page.route("**/api/v1/tokens/renew", (route) => route.fulfill({ status: 204 }));
    await page.goto("/login");
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
    await expect(page.getByRole("heading", { name: "Welcome back" })).toBeVisible();
    await expectNoAccessibilityViolations(page);
  });
}
```

- [ ] **Step 2: Strengthen mobile keyboard navigation coverage**

In the mobile branch, open the menu with keyboard and close it with Escape:

```ts
await menu.focus();
await page.keyboard.press("Enter");
await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeVisible();
await page.keyboard.press("Escape");
await expect(page.getByRole("navigation", { name: "Mobile primary" })).toBeHidden();
await expect(menu).toBeFocused();
```

Keep the existing link-selection focus-restoration path as a separate assertion
in the same test or in a focused mobile-navigation test.

- [ ] **Step 3: Run e2e tests and confirm only expected visual baselines fail**

Run: `mise run frontend:test:e2e`

Expected: Functional, accessibility, overflow, and theme tests PASS; existing
dashboard screenshots FAIL because the approved redesign intentionally changes
pixels.

- [ ] **Step 4: Review and regenerate screenshot baselines**

Inspect Playwright's failed actual/diff artifacts to verify the pages are fully
rendered at 320 and 1440, have no clipped actions, and match the modern-editorial
design. Then run:

```sh
mise run frontend:test:e2e -- --update-snapshots
mise run frontend:test:e2e
```

Expected: snapshots update and the second command PASS. Do not accept snapshots
with missing fonts, loading skeletons, overflow, or accidental blank API states.

- [ ] **Step 5: Update frontend documentation**

Change the opening description in `frontend/README.md` to:

```md
The SimpleBank web UI is a Svelte 5 single-page application built with Vite,
TypeScript, Tailwind CSS, and daisyUI. It uses custom `simplebank-light` and
`simplebank-dark` themes; an explicit selection is stored in the browser, while
the operating-system preference supplies the initial default.
```

Keep all current setup/build/testing guidance intact.

- [ ] **Step 6: Run formatting and the complete frontend completion gates**

First format changed frontend files:

```sh
mise run frontend:format
```

Then run fresh verification:

```sh
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:test
mise run frontend:test:e2e
mise run frontend:build
mise run app:build
```

Expected: every command exits 0. The final app build proves the redesigned Vite
assets remain embeddable in the Go binary.

- [ ] **Step 7: Inspect final diff and dependency scope**

Run:

```sh
git diff --check
git status --short
git diff --stat
git diff -- frontend/package.json frontend/src/app.css frontend/src/lib/theme.ts
```

Expected: no whitespace errors, only intended frontend/docs files changed,
`daisyui` is the only new dependency, and no backend/API files changed. Preserve
unrelated pre-existing worktree files such as `.agents/`.

- [ ] **Step 8: Commit browser proof and documentation**

```sh
git add frontend/e2e/accessibility.spec.ts frontend/e2e/accessibility.spec.ts-snapshots frontend/README.md
git commit -m "test(frontend): verify daisyui redesign"
```

---

## Final Review Checklist

- [ ] Every page and shared visual control uses daisyUI semantic classes.
- [ ] No legacy bespoke semantic color utility remains in `frontend/src`.
- [ ] Both custom themes pass axe checks on representative public and authenticated pages.
- [ ] Theme initialization occurs before `mount`, explicit changes persist, invalid values fall back, and storage errors remain non-fatal.
- [ ] Mobile navigation works by keyboard, Escape restores focus, and links retain modified-click behavior.
- [ ] Transfer idempotency and account-opening policy tests remain unchanged in intent and pass.
- [ ] Session-scoped account reset and stale-response generation tests pass.
- [ ] Existing route title, announcement, and focus tests pass.
- [ ] All completion commands have fresh successful output.
