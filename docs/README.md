# SimpleBank Documentation

Project documentation is organized by purpose. Start with the current guides
and use architecture decision records for durable rationale.

## Current Guides

- [Project README](../README.md): setup, commands, configuration, API, and
  current architecture.
- [Frontend README](../frontend/README.md): Svelte, Tailwind CSS, and daisyUI
  architecture; custom light/dark themes; local UI development; build and
  embedding behavior; and frontend verification.
- [Security guide](security.md): implemented controls, production deployment
  checklist, credential migration impact, and known limitations.

These guides describe the repository as it works today and should change with
the behavior they document.

## Toolchain

The project currently targets Go 1.27 and uses its standard-library `uuid`
package. Exact Go, Bun, and development-tool versions are pinned in
[`mise.toml`](../mise.toml) and checksummed in [`mise.lock`](../mise.lock); run
`mise install` to install them.

The web UI targets Svelte 5, TypeScript 6, Tailwind CSS 4, and daisyUI 5. The
frontend defines only the custom `simplebank-light` and `simplebank-dark`
themes, applies the selected theme before Svelte mounts, and stores an explicit
selection in browser local storage.

## Architecture Decisions

[Architecture decision records](decisions/README.md) capture significant,
hard-to-reverse choices and their trade-offs. Accepted ADRs are authoritative
for the decisions they cover. Do not delete an outdated ADR; add a new record
that supersedes it and update both records' statuses.

## Updating Documentation

- Update the relevant current guide when setup, commands, configuration, API
  behavior, test workflows, or repository structure changes.
- Write an ADR for a significant decision that would be expensive to reverse;
  do not use ADRs for routine implementation details.
- Prefer links to a single source of truth over duplicating detailed behavior
  across documents.
