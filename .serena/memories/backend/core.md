# Backend

- `cmd/app` is the composition root: CLI/configuration, migrations, shared dependencies, River startup, HTTP startup, and ordered shutdown.
- `internal/api`: Echo server owns routing; readiness is injected. API base `/api/v1`; `/livez` and `/readyz` unversioned.
- `internal/config`: CLI flag/environment loading and validation. Required runtime values: `DB_SOURCE`, JWT secret >=32 chars, `SMTP_FROM`.
- `internal/db`: wide sqlc-backed `Store` interface plus handwritten transaction methods; migrations run automatically at startup before services.
- Account opening invariant: `CreateAccountTx` creates the account and a matching entry for non-zero opening balances, preserving `balance == SUM(entries)` for reconciliation. Configured opening caps are enforced by the backend and exposed through public policy metadata.
- Transfer invariant: one DB transaction; deterministic account lock order; locked-row currency revalidation; configured limits; guarded balance update; client UUID idempotency key collapses retries.
- Transfer request privacy: validate and authorize the source account before looking up the destination, so an unauthorized caller cannot probe destination-account existence.
- Refresh tokens are hashed at rest and transported only by HttpOnly same-site cookie; access tokens use Bearer auth.
- River worker runs in the HTTP process. Start River before accepting HTTP; stop HTTP before bounded River shutdown; close the database last. Worker count is per replica.
- `internal/mail` owns mailer interface/SMTP adapter; `internal/password` bcrypt; `internal/token` maker/JWT; `internal/secret` crypto tokens; `internal/random` non-crypto data.
- Key rationale: `docs/decisions/0001` through `0006`.