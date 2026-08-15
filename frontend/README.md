# SimpleBank Frontend

The SimpleBank web UI is a Svelte 5 single-page application built with Vite,
TypeScript, and Tailwind CSS. Production assets are embedded in the Go binary,
so the UI and `/api/v1` are served from the same origin.

See the [project README](../README.md) for the complete application setup,
configuration, API, and deployment-oriented Compose profiles.

## Local Development

Run these commands from the repository root:

```sh
mise install
mise run frontend:install
mise run compose:dev:up
mise run frontend:dev
```

Open http://localhost:5173 for Vite hot-module replacement. The development
server proxies `/api`, `/livez`, and `/readyz` to the Go server at
http://localhost:8080, matching the same-origin behavior of the production
binary.

Use `mise run compose:dev:down` when finished. The full dev stack remains
available directly at http://localhost:8080 using the last embedded frontend
build.

## Commands

| Command | Description |
|---------|-------------|
| `mise run frontend:install` | Install locked dependencies with Bun |
| `mise run frontend:dev` | Start the Vite development server |
| `mise run frontend:build` | Build production assets into `frontend/dist` |
| `mise run frontend:preview` | Preview the production frontend build |
| `mise run frontend:check` | Run `svelte-check` and TypeScript checks |
| `mise run frontend:lint` | Lint Svelte and TypeScript sources with ESLint |
| `mise run frontend:format:check` | Check formatting with Prettier |
| `mise run frontend:test` | Run Vitest unit tests once |
| `mise run frontend:test:e2e` | Run Playwright responsive accessibility tests |
| `mise run frontend:test:e2e:install` | Install Chromium for Playwright |

The package also provides watch, coverage, auto-fix, and formatting scripts for
interactive use; see `package.json` for the complete list.

## Build and Serving

`mise run frontend:build` writes Vite's production output to `frontend/dist`.
The `frontend` Go package embeds that directory, and `mise run app:build`
depends on the frontend build so the binary always contains current assets.

The Go server registers API and health routes before the SPA catch-all. Unknown
non-API paths receive `index.html` for client-side routing, while unknown
`/api` paths remain JSON 404 responses. Content-hashed assets are cached as
immutable; the app shell is always revalidated.

## Testing

Vitest and Testing Library cover component behavior in jsdom. Playwright runs
browser-level checks at desktop and mobile viewports, with axe assertions for
accessibility regressions.

Before shipping a UI change, run:

```sh
mise run frontend:check
mise run frontend:lint
mise run frontend:format:check
mise run frontend:test
mise run frontend:test:e2e
```
