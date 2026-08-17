# Technology Stack

- Module: `github.com/vancanhuit/simplebank`; `go.mod` language version 1.26.5; mise pins Go 1.26.6.
- Backend: Echo v5 HTTP, urfave/cli v3, pgx v5/PostgreSQL, sqlc, goose migrations, River jobs, JWT v5, validator v10, bcrypt via `x/crypto`.
- Frontend: Svelte 5, TypeScript 6, Vite 8, Tailwind CSS 4, Bun 1.3.14.
- Frontend verification: Vitest/jsdom + Testing Library; Playwright + axe; ESLint 10; Prettier 3.
- Infra: Docker Compose, PostgreSQL, Mailpit, pgAdmin; Caddy reverse proxy; mkcert local TLS.
- Tool manager/task runner: mise (`auto_install=false`, lockfile enabled). Pins include golangci-lint 2.12.2, cocogitto 7.0.0, mkcert 1.4.4.
- Production artifact: CGO-disabled Go binary containing Vite output from `frontend/dist`.