# daisyUI Frontend Redesign

## Summary

Redesign the SimpleBank Svelte SPA with daisyUI 5 while preserving its existing
application behavior, API contracts, routing, security properties, and product
rules. The new interface will use a custom modern-editorial visual system with
paired light and dark themes, clearer content hierarchy, and consistent daisyUI
components across public and authenticated pages.

This is a frontend-only UX change. It does not alter backend APIs, domain rules,
authentication behavior, authorization, transfer semantics, or persisted
banking data.

## Goals

- Use daisyUI 5 as the frontend component styling system.
- Replace bespoke component styling with canonical daisyUI component classes.
- Establish custom `simplebank-light` and `simplebank-dark` themes.
- Redesign all current pages with a distinctive modern-editorial banking style.
- Improve responsive hierarchy, navigation, forms, loading states, empty states,
  retry paths, and feedback without changing product workflows.
- Preserve or improve accessibility, keyboard operation, reduced-motion support,
  forced-color behavior, and minimum touch targets.
- Keep application logic in the existing Svelte components, stores, router, and
  API client rather than coupling behavior to the styling library.

## Non-Goals

- No backend, database, endpoint, authorization, or API contract changes.
- No changes to transfer validation, source-scoped idempotency, currency rules,
  account-opening limits, or account session isolation.
- No new product workflows, pages, account capabilities, or banking features.
- No migration to a different router, state library, icon library, or framework.
- No generic abstraction layer over daisyUI and no speculative component system.

## Architecture

The application remains a Svelte 5 SPA using Vite, Tailwind CSS 4, and the
current handwritten router and stores. daisyUI is installed as a development
dependency and configured through the Tailwind 4 CSS plugin syntax in
`frontend/src/app.css`.

Responsibility remains divided as follows:

- daisyUI owns component appearance and semantic theme tokens.
- Tailwind utilities own page layout, responsive composition, spacing, and the
  few accessibility variants that do not belong to a daisyUI component.
- Svelte shared components own application behavior and accessible semantics.
- Existing stores, router, API client, and page scripts continue to own state and
  workflows.

The redesign will not wrap every daisyUI component. Existing wrappers remain
only where they already provide application behavior:

- `Button.svelte` owns loading, disabled, type, and click behavior.
- `TextField.svelte` owns label association, hints, errors, described-by links,
  bindable values, and focus when a new validation error appears.
- `Alert.svelte` owns assertive versus polite live-region semantics.
- `Link.svelte` owns in-app navigation and modified-click behavior.

Their visual implementations will use canonical classes such as `btn`, `input`,
`label`, and `alert` instead of reproducing those controls with long utility
strings.

## Theme System

Two custom daisyUI themes will be defined:

- `simplebank-light`: warm neutral base surfaces, dark ink, and concentrated
  teal/cyan primary accents.
- `simplebank-dark`: deep blue-green neutral surfaces with contrast-equivalent
  semantic colors and restrained elevation.

Both themes will define the complete daisyUI semantic palette, including base,
primary, secondary, accent, neutral, info, success, warning, and error colors,
plus matching content colors. Radius, border, and depth values will support the
editorial direction without making the interface overly soft or decorative.

IBM Plex Sans remains the primary typeface. Large page headings, strong monetary
figures, generous whitespace, tabular numerals, and restrained bordered surfaces
will establish hierarchy. Theme tokens replace the current bespoke color
utilities such as `brand`, `surface`, `ink`, and `muted`; layout utilities remain
where they are clearer than custom CSS.

## Theme Behavior

A small frontend theme module will own preference handling. It will:

1. Accept only the two supported theme names.
2. Read a saved preference from local storage.
3. Fall back to the operating-system color preference when no valid preference
   exists.
4. Apply the selected theme through the root document's `data-theme` attribute.
5. Persist explicit user changes.
6. Expose the current theme and a toggle operation to the header.

Theme initialization runs before the Svelte app mounts so the first rendered
frame uses the correct theme. Storage failures or malformed values are
non-fatal and fall back to system preference. Theme preference is independent
of authentication and must not interact with session-scoped account state.

The authenticated header will include an accessible icon-and-label theme toggle
with a stable accessible name describing the resulting action. The control must
remain usable on desktop and mobile and must not rely on hover interaction.

## Shared Components

### Brand Mark

The repeated SimpleBank logo markup in the authenticated header and public auth
layout will become one small shared component. It will support the size and text
treatment needed in both locations while preserving semantic link ownership in
the caller.

### Buttons And Links

Button variants map directly to daisyUI button modifiers. Loading state uses a
daisyUI loading indicator while preserving `aria-busy`, disabled behavior, and
button text. Link buttons use the same daisyUI vocabulary at call sites so
actions have consistent visual priority without changing SPA navigation.

### Fields And Choice Controls

Text fields use daisyUI fieldset, label, input, and helper/error conventions.
The current id generation, label association, input attributes, hints,
validation errors, and focus behavior remain intact. Select and radio controls
on transfer and account-opening pages adopt daisyUI classes and continue to use
native form elements.

### Alerts And Feedback

Alerts use daisyUI semantic variants with suitable icons where useful. Error
alerts retain `role="alert"`; success and informational notices retain polite
status behavior. Inline retries remain keyboard-accessible actions with clear
focus and touch targets.

### Cards, Stats, Skeletons, And Empty States

Cards use daisyUI card structure where content has a clear card boundary.
Currency summaries use daisyUI stats with responsive wrapping. Loading
placeholders use skeleton classes with meaningful busy labels. Empty states use
consistent card and action treatments rather than ad hoc dashed containers.

## Page Design

### Application Shell

The authenticated shell uses a responsive daisyUI navbar with:

- Shared SimpleBank branding.
- Clearly visible active navigation for Overview and Transfer.
- User identity at wider breakpoints.
- Theme control and sign-out actions.
- Explicit mobile navigation with Escape handling and focus restoration.

The existing skip link, main landmark, route title updates, route announcement,
and post-navigation focus management remain unchanged in behavior. The footer is
restyled with theme-aware semantic colors and its technology list includes
daisyUI.

### Public Authentication

Login, registration, and email verification use a split editorial composition
on wider screens, pairing concise brand/value copy with a focused form or status
card. Mobile collapses to a single-column layout without decorative content
preceding the primary task. Existing submit, registration notice, verification,
redirect, and error behavior remains unchanged.

### Dashboard

The dashboard leads with a stronger financial summary containing the greeting,
account count, primary actions, and one stat per held currency. Account cards
show balance first, then currency, opening date, account identifier, copy status,
and transfer/activity actions in a consistent hierarchy.

Initial loading uses responsive skeleton cards. API failures retain a visible
retry action. The no-account state provides one clear account-opening action.
The unverified-email notice remains visible but does not compete with the main
balance summary.

### Transfer

The transfer page becomes a focused card-based form with a consistent page
heading and back-navigation pattern. Source account selection exposes currency
and current balance clearly. Recipient and amount errors remain attached to the
correct fields. The successful receipt becomes a structured status card that
emphasizes amount, remaining balance, and reference.

All transfer invariants remain unchanged, including stable idempotency keys
across retries, key rotation only after confirmed success, same-account
validation, policy-derived limits, and balance refresh after transfer.

### New Account

The account-opening page uses a focused form card. Available currencies remain
native radio choices but become clearly selectable daisyUI cards. Loading,
failure, and retry states for the opening policy remain prominent, and the form
remains disabled until policy data is ready. Existing currency availability and
opening-deposit validation remain unchanged.

### Account Activity

The account header emphasizes current balance and account identity. Transfers
render as a responsive list with distinct incoming and outgoing indicators,
counterparty, date, and signed amount. Loading and empty states use the shared
skeleton and card vocabulary. The page retains stale-request generation guards
and deep-link loading behavior.

### Not Found

The not-found page uses the same editorial heading and action vocabulary as the
rest of the app, with a single destination appropriate to authentication state.

## Accessibility

The redesign must retain or improve:

- Semantic landmarks and heading order.
- Persistent SPA route announcements and focus movement.
- A keyboard-visible skip link.
- Label and description associations for all form controls.
- Assertive announcements for errors and polite announcements for status.
- Keyboard operation and Escape/focus restoration for mobile navigation.
- Stable accessible names for icon-only controls.
- At least 44-pixel interactive targets where controls are intended for touch.
- Sufficient text, control, and state contrast in both custom themes.
- Reduced-motion behavior for spinners, skeletons, and transitions.
- Forced-color usability for active, invalid, and focused states.
- Responsive layouts without horizontal overflow at tested mobile widths.

daisyUI classes do not replace native semantics. Native buttons, links, inputs,
selects, radios, fieldsets, legends, lists, and landmarks remain the underlying
elements.

## Preserved Invariants

The implementation must not change:

- Authentication initialization, route guards, or public verification access.
- Account reset generation checks that prevent stale responses crossing login
  sessions.
- Logout cleanup, including cleanup when the network request fails.
- Source-scoped transfer intent and idempotency behavior.
- Client-side checks that mirror server transfer and account-opening policies.
- The API as the authority for all limits and banking validation.
- Verification code removal from browser history after capture.
- Modified-click and new-tab behavior for links.
- API and health route precedence over the production SPA fallback.

## Error Handling

Existing API error conversion remains authoritative. The redesign changes how
errors are presented, not how they are produced. Theme storage access is wrapped
so unavailable local storage cannot prevent application startup. A rejected
theme value is ignored rather than applied to the document.

Loading and retry affordances must remain local to the operation that failed.
The interface must not imply success before an API operation confirms it, and
loading controls must prevent duplicate submissions where they do today.

## Testing

### Unit And Component Tests

- Preserve behavioral coverage for auth, routing, stores, forms, transfers,
  account creation, account history, alerts, fields, buttons, and account cards.
- Update styling assertions only where daisyUI classes form part of the shared
  component contract.
- Add focused tests for valid saved theme initialization, system-preference
  fallback, invalid preference fallback, toggle behavior, persistence, and
  storage failure.
- Verify the theme control's accessible name and state changes.
- Preserve accessible roles, names, descriptions, and busy/disabled assertions.

### Browser Tests

- Exercise authenticated desktop and mobile navigation.
- Verify mobile menu keyboard behavior and focus restoration.
- Exercise both custom themes and ensure preference survives reload.
- Check representative public, dashboard, form, receipt, loading, error, and
  empty states for responsive overflow.
- Run axe assertions in both themes on representative pages.
- Keep existing rate-limit and security-facing browser expectations intact.

### Completion Gates

Run the repository's frontend checks:

```sh
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:test
mise run frontend:test:e2e
mise run frontend:build
mise run app:build
```

The production and app builds confirm that daisyUI is compiled into the SPA and
that the resulting assets still embed in the Go binary.

## Documentation

Update `frontend/README.md` to identify daisyUI as the component styling layer,
document the custom themes, and retain the existing development and verification
commands. Update the footer technology list to include daisyUI.

## Acceptance Criteria

- Every existing page uses the custom daisyUI visual system.
- No legacy bespoke semantic color utilities remain in page or component markup.
- Light and dark themes are accessible, user-selectable, and persisted.
- The correct theme is applied before application mount.
- All existing frontend workflows and preserved invariants continue to pass.
- Shared behavioral wrappers use daisyUI classes without duplicating daisyUI's
  component implementation.
- Desktop and mobile layouts are complete, keyboard-operable, and free of
  unintended horizontal overflow.
- Unit, accessibility, e2e, frontend build, and app build gates pass.
