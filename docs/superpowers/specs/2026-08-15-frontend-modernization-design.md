# Frontend Modernization Design

**Date:** 2026-08-15
**Status:** Approved

## Objective

Improve SimpleBank's existing frontend without replacing its architecture or
visual identity. The work fixes two proven correctness risks, completes focused
Svelte 5 modernization, refines the Tailwind CSS 4 design system, and strengthens
SPA behavior and browser verification.

## Current State

The frontend is a Vite 8 SPA using Svelte 5 runes, a small History API router,
Tailwind CSS 4 through the Vite plugin, and an embedded Go production server.
Its routes, stores, forms, loading states, and accessibility baseline are
coherent. The Tailwind integration already follows the CSS-first v4 model with
`@import "tailwindcss"` and `@theme`. Components already use `$state`,
`$derived`, `$effect`, `$props`, callback props, snippets, and `mount`; no legacy
or deprecated Svelte APIs remain.

The audit found these concrete gaps:

- simultaneous authenticated requests that receive `401` each start a refresh;
  refresh credentials rotate once, so only one renewal succeeds and another
  failure can clear valid client authentication;
- Go exposes monetary `int64` values as JSON numbers while PostgreSQL permits
  balances and transfer amounts above JavaScript's safe-integer range;
- the custom router has no direct coverage for push navigation, back/forward
  navigation, deep links, or routed title and focus behavior;
- the account-opening page uses an effect to synchronize selected currency with
  loaded account data when direct initialization logic is clearer;
- the global font declaration names Inter without bundling it, so appearance
  varies with host fonts;
- the visual system is accessible but can provide stronger hierarchy, denser
  scanning, and more consistent state and action treatment.

## Goals

- Make access-token renewal single-flight across concurrent failed requests.
- Guarantee every monetary JSON number consumed by the SPA is an exact
  JavaScript integer.
- Keep all Svelte components in modern runes mode and avoid deprecated APIs.
- Test custom routing and SPA navigation semantics directly.
- Keep the current restrained banking identity while improving typography,
  hierarchy, responsiveness, and interaction clarity.
- Use Tailwind CSS 4's CSS-first tokens and current variants rather than legacy
  JavaScript configuration or deprecated functions.
- Preserve WCAG 2.1 AA behavior, keyboard operation, reduced motion, and
  supported layouts from 320px through wide desktop.

## Non-Goals

- Migrating to SvelteKit or another router.
- Replacing the existing API, store, or authentication architecture.
- Changing routes, user workflows, transfer semantics, or authorization rules.
- Adding dark mode, charts, analytics, marketing pages, or speculative product
  features.
- Introducing a broad UI framework or a generic component abstraction layer.
- Refactoring unrelated backend packages.

## Architecture Decision

Keep the current Vite SPA, custom router, rune-backed stores, and embedded
deployment. Deliver changes in four independently testable slices:

1. frontend and API correctness;
2. Svelte 5 lifecycle and router coverage;
3. Tailwind 4 design-system and page refinements;
4. browser-level responsive and accessibility verification.

This preserves working ownership boundaries. The API continues to own transport
validation, stores own remote state, pages own workflows, and presentation
components own reusable controls and account display.

## Concurrent Session Renewal

The API client owns one module-scoped in-flight renewal promise. When an
authenticated request receives `401`, it joins that promise instead of calling
`auth.tryRefresh` independently. The first caller creates the promise; all
concurrent callers await the same result; cleanup clears the shared reference
after settlement so a later expiry can renew again.

Each original request retries at most once after a successful shared renewal.
Failed renewal retains existing behavior: authentication is cleared and each
caller receives its original unauthorized outcome. This change does not weaken
the server's one-time refresh-token rotation or reuse rejection.

Focused tests must prove:

- two concurrent `401` responses invoke one renewal;
- both requests retry after successful renewal;
- failed shared renewal does not retry either request;
- a later, separate `401` can start a new renewal.

## Exact Money Boundary

Keep the existing `/api/v1` JSON number contract. Changing established account,
transfer, and limit fields to decimal strings would be a breaking API change and
would require response adapters across every monetary endpoint. Instead, make
the current contract honest by enforcing JavaScript's maximum safe integer,
`9,007,199,254,740,991`, as the service-wide maximum monetary minor-unit value.

The backend must reject:

- account-opening and transfer-limit configuration above the safe bound;
- transfer requests above the safe bound, even when a configured transfer limit
  is disabled;
- a credit that would raise the destination balance above the safe bound.

Database transaction logic remains authoritative for the balance ceiling so
concurrent credits cannot cross it. API validation provides an early,
client-safe rejection for oversized input. Existing non-negative balance,
positive transfer, configured opening-balance, and per-currency transfer checks
remain unchanged.

The migration adds a database check for stored account balances. Existing data
must be checked before applying it; migration rollback removes only that check.
No data rewrite or JSON field change occurs. Frontend money remains represented
as `number`, and existing `Number.isSafeInteger` input validation remains valid.

Tests cover the exact bound and one unit above it at configuration, handler, and
transaction boundaries. An integration test proves concurrent or sequential
credits cannot produce an unsafe account balance.

## Svelte 5 Design

All authored components remain in runes mode. New or changed code uses:

- `$state` for mutable local state;
- `$derived` or `$derived.by` for pure derived values;
- `$props` and `$bindable` for component contracts;
- callback event properties instead of `on:` directives or event dispatchers;
- snippets and `{@render}` instead of slots;
- direct dynamic component rendering instead of `<svelte:component>`;
- `mount` for application startup.

Effects remain only for browser side effects such as document title, focus,
history-linked UI, and teardown-bearing subscriptions. The account-opening
currency correction moves from an effect into account-load initialization,
because it synchronizes form state rather than an external system.

The current `onMount` data loads remain appropriate: they start browser-only
remote work after component mount and do not represent deprecated lifecycle
usage. Existing timer teardown in account cards remains unchanged.

## Router and SPA Semantics

Keep the small History API router because the route set is fixed and the Go
server already supports SPA deep links. Add focused tests around its public
behavior rather than replacing it with a dependency.

Tests cover:

- path normalization and `pushState` navigation;
- `popstate` updates for browser back and forward actions;
- protected and public deep-link resolution;
- authenticated redirects;
- document-title updates after navigation;
- route announcements and focus transfer to the new main region.

The shell retains one persistent live region and one primary main landmark.
Navigation must preserve history behavior, current-link semantics, skip-link
operation, and keyboard focus restoration for the mobile disclosure.

## Tailwind CSS 4 Design System

Continue using the official Vite plugin and `@import "tailwindcss"`. Keep theme
configuration in CSS with top-level `@theme`; do not add `tailwind.config.js`,
`@config`, deprecated `theme()`, or dynamically constructed utility names.

Refine semantic tokens around these roles:

- canvas, surface, raised surface, border, control, ink, and muted text;
- brand, strong brand, and soft brand;
- positive, negative, information, and amber attention states;
- application font, card radius, and restrained elevation.

Use OKLCH colors for perceptual consistency while retaining the current teal
identity. Teal remains the action and trust color; cool neutrals carry most
surface area; amber is reserved for attention and pending states. Cards use a
maximum 8px radius. Shadows remain subtle and only distinguish genuine raised
controls or overlays.

Bundle IBM Plex Sans Variable locally through Fontsource and expose it through a
Tailwind `--font-sans` theme token. Account numbers and technical references
retain the system monospace stack. The font must not require a runtime network
request. Use `@lucide/svelte` for standard interface icons introduced or touched
by this work; keep the existing bank mark as a product-specific graphic.

Use current Tailwind variants for interactive and environmental states,
including `aria-*`, `has-*`, `motion-reduce`, `contrast-more`, and
`forced-colors` where they improve behavior. Keep complete class names visible
to source detection.

## SPA Visual and Interaction Design

The application remains a quiet operational banking tool, not a marketing
site. Pages prioritize balance scanning, account identification, and repeated
actions.

### Application shell

Retain the compact desktop header and mobile disclosure navigation. Improve
active-state contrast, icon consistency, identity truncation, and separation
between navigation and session action. Do not add a decorative sidebar or
mobile bottom navigation for only two destinations.

### Dashboard

Keep the summary band as the first content region. Strengthen balance hierarchy
and responsive wrapping without increasing hero scale. Keep account cards as
individual repeated records, not nested inside another card. Make primary and
secondary actions visually distinct and stable at 320px.

### Forms

Keep transfer and account-opening forms narrow and task-focused. Standardize
page heading, back navigation, field spacing, policy messages, button width,
and receipt presentation. Preserve first-invalid-field focus and explicit
error association.

### Account activity

Keep the responsive list rather than introducing a desktop-only table. Improve
counterparty readability, signed amount alignment, status distinction, and
small-screen wrapping while retaining semantic list markup.

### States and motion

Loading, empty, error, success, and disabled states use consistent spacing,
type, and semantic color roles. All interactive targets remain at least 44px.
Motion is limited to disclosure and state transitions, and reduced-motion users
receive effectively static behavior. Forced-colors mode must retain visible
borders, focus, selection, and status meaning.

## Dependency Policy

Add only two focused frontend packages:

- `@fontsource-variable/ibm-plex-sans` for deterministic local typography;
- `@lucide/svelte` for maintained, accessible interface icon components.

Both must be pinned through the existing Bun lockfile, imported selectively,
and reviewed for license, maintenance, and bundle impact. No runtime CSS or icon
CDN is permitted.

## Testing and Verification

Implementation follows failing-test-first slices.

### Focused tests

- Vitest tests for shared renewal and retry behavior.
- Go unit and integration tests for the safe monetary ceiling.
- Router and application tests for history, deep links, title, announcement,
  and focus behavior.
- Component tests for any changed control semantics.

### Browser tests

Playwright verifies 320x800, 768x900, 1024x900, and 1440x1000 viewports. Tests
cover authenticated dashboard, mobile navigation, transfer validation, account
activity, browser back/forward navigation, keyboard operation, and absence of
horizontal overflow. Axe runs on representative states.

After visual changes, capture desktop and mobile screenshots and inspect them
for nonblank content, text clipping, overlap, focus visibility, layout shift,
and actual font loading. Browser console and network checks must show no runtime
errors or missing assets.

### Repository gates

- `mise run frontend:test`
- `mise run frontend:check`
- `mise run frontend:lint`
- `mise run frontend:format:check`
- `mise run frontend:build`
- `mise run frontend:test:e2e`
- focused Go tests for changed API, configuration, and transaction packages
- `mise run test:unit`
- `mise run golangci-lint`

## Rollout and Reversibility

Frontend and backend ship together in one embedded binary, so shared-renewal,
safe-money validation, and UI changes cannot drift across independent frontend
deployments. The monetary database check must be deployed only after confirming
all existing balances satisfy the safe bound. Its down migration removes the
check without changing data.

Visual changes are CSS and component-only and can be reverted independently of
correctness changes. No feature flag is needed because routes and workflows do
not change.

## Alternatives Rejected

### Convert all monetary JSON fields to decimal strings

Strings would support the full `int64` range, but changing existing fields is a
breaking `/api/v1` contract. An additive dual-field migration would duplicate
every monetary response and complicate limits endpoints. SimpleBank does not
need balances above the safe bound, so an enforced invariant is smaller and
keeps current clients compatible.

### Adopt SvelteKit or a routing package

The application has a small fixed route set and already supports production
deep links through the Go SPA fallback. Replacing routing adds migration and
bundle cost without solving a current product need. Direct tests provide the
missing confidence.

### Full visual redesign

The current interface already has coherent workflows and accessibility work.
A new shell, navigation model, or component framework would create broad churn
and obscure the correctness fixes. Focused refinement delivers clearer value.

### Add dark mode

Dark mode doubles token and state verification scope without serving the
requested modernization goals. The light operational theme remains explicit.

## Official References

- Svelte 5 migration guide:
  https://svelte.dev/docs/svelte/v5-migration-guide
- Svelte `$state`: https://svelte.dev/docs/svelte/$state
- Svelte `$derived`: https://svelte.dev/docs/svelte/$derived
- Svelte `$effect`: https://svelte.dev/docs/svelte/$effect
- Svelte snippets: https://svelte.dev/docs/svelte/snippet
- Tailwind CSS Vite installation:
  https://tailwindcss.com/docs/installation/using-vite
- Tailwind CSS theme variables: https://tailwindcss.com/docs/theme
- Tailwind CSS functions and directives:
  https://tailwindcss.com/docs/functions-and-directives
- Tailwind CSS source detection:
  https://tailwindcss.com/docs/detecting-classes-in-source-files
- Tailwind CSS states and variants:
  https://tailwindcss.com/docs/hover-focus-and-other-states
- Lucide for Svelte: https://lucide.dev/guide/svelte/getting-started
- IBM Plex Sans Variable installation:
  https://fontsource.org/fonts/ibm-plex-sans/install

## Acceptance Criteria

- Concurrent authenticated `401` responses produce exactly one refresh attempt
  and all eligible requests retry once after success.
- Every monetary value returned to or accepted from the SPA is within
  JavaScript's safe-integer range, enforced by API and database boundaries.
- No changed Svelte file uses legacy reactivity, event directives, event
  dispatchers, slots, class components, or deprecated component types.
- Tailwind remains CSS-first with no legacy configuration or deprecated
  `theme()` usage.
- Typography loads locally and consistently; standard touched icons use
  Lucide components.
- Existing routes and workflows remain intact, including deep links,
  back/forward navigation, title updates, announcements, and focus movement.
- Dashboard, forms, mobile navigation, and account activity remain usable and
  free from horizontal overflow at all supported viewports.
- Axe reports no violations on representative authenticated and public states.
- Focused tests, full frontend gates, browser tests, Go tests, and lint pass.
