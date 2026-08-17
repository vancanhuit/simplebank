# Frontend

- Svelte 5 SPA under `frontend/src`; production output `frontend/dist` is embedded by `frontend/embed.go` into the Go binary.
- `mise run app` and `app:build` depend on `frontend:build`, so `dist` exists and current assets compile into the binary.
- Production uses same origin for SPA and `/api/v1`. Server registers API/health before SPA catch-all; unknown non-API routes return `index.html`, unknown `/api` routes remain JSON 404.
- Vite development server at port 5173 proxies `/api`, `/livez`, `/readyz` to backend at `http://localhost:8080`.
- Auth-scoped stores must reset when auth is lost. `AccountsStore.reset` increments a generation and clears all account/UI state; async load/create completions may mutate the cache only when their captured generation is still current, preventing stale data from crossing sessions.
- Build-time `VERSION` becomes `__APP_VERSION__`, defaulting to `dev` for plain Vite runs.
- Unit tests: Vitest/jsdom + Testing Library, matching `src/**/*.{test,spec}.{ts,js}`.
- Browser tests: Playwright desktop/mobile viewports with axe accessibility assertions.
- Content-hashed assets are immutable-cached; app shell is always revalidated.