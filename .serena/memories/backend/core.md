# Backend

- `cmd/app/main.go`: CLI entrypoint and dependency assembly. `serve` builds config/pool/store/mailer/River/API/SPA.
- `internal/api`: Echo server owns routing; readiness is injected. API base `/api/v1`; `/livez` and `/readyz` unversioned.
- `internal/config`: CLI flag/environment loading and validation. Required runtime values: `DB_SOURCE`, JWT secret >=32 chars, `SMTP_FROM`.
- `internal/db`: wide sqlc-backed `Store` interface plus handwritten transaction methods; migrations run automatically at startup before services.
- Transfer invariant: one DB transaction; deterministic account lock order; locked-row currency revalidation; configured limits; guarded balance update; client UUID idempotency key collapses retries.
- Refresh tokens are hashed at rest and transported only by HttpOnly same-site cookie; access tokens use Bearer auth.
- River worker runs in same process as HTTP. Start worker before accepting HTTP; stop HTTP before bounded worker shutdown. Worker count is per replica.
- `internal/mail` owns mailer interface/SMTP adapter; `internal/password` bcrypt; `internal/token` maker/JWT; `internal/secret` crypto tokens; `internal/random` non-crypto data.
- Key rationale: `docs/decisions/0001` through `0006`.