# Suggested Commands

Run from repository root.

- Install pinned toolchain: `mise install`.
- Full dev stack: `mise run compose:dev:up`; stop/remove volumes: `mise run compose:dev:down`.
- Direct HTTPS: `mise run compose:dev-https:up`; proxy TLS: `mise run compose:dev-proxy:up`; matching `:down` tasks remove volumes.
- Run CLI after required env (`DB_SOURCE`, `JWT_SECRET`, `SMTP_FROM`): `mise run app -- serve`; other subcommands: `healthcheck`, `version`.
- Build embedded binary: `mise run app:build` -> `dist/simplebank`.
- Frontend HMR: `mise run frontend:dev` (http://localhost:5173); production preview: `mise run frontend:preview`.
- Frontend dependencies/build: `mise run frontend:install`; `mise run frontend:build`.
- Unit tests: `mise run test:unit`; integration tests with managed PostgreSQL: `mise run test:integration`.
- Go lint/format: `mise run golangci-lint`; `mise run golangci-lint:fmt`.
- Frontend checks: `mise run frontend:check`; `mise run frontend:lint`; `mise run frontend:format:check`; `mise run frontend:test`; `mise run frontend:test:e2e`.
- Install Playwright Chromium once: `mise run frontend:test:e2e:install`.
- Regenerate DB query code: `mise run sqlc:generate`.
- Vulnerability scan: `mise run govulncheck`; Docker image: `mise run docker:build`; git hooks: `mise run hooks:install`.
- Service URLs: app/API http://localhost:8080; Mailpit http://localhost:8025; pgAdmin http://localhost:5050.