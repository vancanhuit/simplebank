# SimpleBank

- Cloud-native banking service: users, JWT auth, accounts, atomic/idempotent transfers, email verification, embedded SPA.
- Source map: `cmd/app` CLI/composition root; `internal` Go backend; `frontend` Svelte SPA; `docs/decisions` accepted ADRs; `caddy` proxy profile; `compose.yaml` local/test services.
- Current guides (`README.md`, `frontend/README.md`) outrank historical `docs/superpowers/{specs,plans}`. Accepted ADRs are authoritative for durable decisions; supersede rather than rewrite/delete.
- Runtime, dependency, and tool pins: `mem:tech_stack`.
- Root-level dev/build/test commands and service URLs: `mem:suggested_commands`.
- Formatting, generated-code, documentation, and commit rules: `mem:conventions`.
- Required verification by change type: `mem:task_completion`.
- Go backend ownership and safety invariants: `mem:backend/core`.
- SPA build, embedding, proxy, and test boundaries: `mem:frontend/core`.