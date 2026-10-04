# Go backend and Svelte frontend audit

## Executive summary

**Status: audit complete within the scope and limitations below.** Nine actionable findings were confirmed by runtime evidence or complete source reasoning. The highest-priority result is mixed-principal notification state after a successful identity-changing refresh. Existing automated checks all passed; that coverage does not include several cross-layer and response-ordering cases identified here.

- Audit date: 2026-09-19.
- Source revision: `bcc511a7b82a79b802e1fba2da6e3e3c62180522`.
- Initial working tree: clean (`git status --short` returned no entries).
- Authorization: comprehensive, report-only audit. Findings will become recommendations for separately authorized changes; application code and tests are not modified.
- Scope: Go backend, Svelte frontend, shared API/database/session/notification invariants, accessibility, performance, dependencies, and repository build/deployment controls.
- Result: 1 medium and 8 low-priority findings; no confirmed critical/high-severity issue or server authorization bypass. Source-confirmed findings are explicitly distinguished from runtime reproductions. Severity reflects demonstrated impact rather than the banking label alone.
- Remaining uncertainty: a logout/late-transfer race, page-initialization races, field performance, real TLS/proxy behavior, and per-platform container artifacts require follow-up evidence. No prior compatible audit ledger was used, and this run does not exhaust possible defects.

## Baseline and governing decisions

Review against current code, [project README](../../README.md), [frontend README](../../frontend/README.md), [security guide](../security.md), and accepted decisions:

- [ADR-0001: wide Store](../decisions/0001-wide-sqlc-backed-store-interface.md).
- [ADR-0002: refresh-token digests](../decisions/0002-hash-refresh-tokens-at-rest.md).
- [ADR-0003: server-owned routing](../decisions/0003-server-owns-routing-with-injected-readiness.md).
- [ADR-0004: domain packages and randomness](../decisions/0004-split-util-into-domain-packages.md).
- [ADR-0005: transfer safety](../decisions/0005-transfer-safety-idempotency-and-limits.md).
- [ADR-0006: worker lifecycle](../decisions/0006-run-worker-with-http-server.md).
- [ADR-0007: durable notifications and SSE](../decisions/0007-deliver-durable-balance-notifications-with-sse.md).

## Environment and command ledger

All commands below ran from the repository root unless noted otherwise. Results refer to this audit run, not prior planning inspection. Generated assets and local caches are not application changes.

| Command | Outcome | Evidence |
| --- | --- | --- |
| `git status --short` | Pass | Initial tree clean. |
| `git rev-parse HEAD` | Pass | Revision recorded above. |
| `mise ls --current` | Pass | Project pins installed: Go 1.27.1, Bun 1.4.2, golangci-lint 2.13.2, cocogitto 7.0.0, mkcert 1.4.4. |
| `mise run frontend:install` | Pass | Frozen install checked 263 installs across 312 packages; no changes. |
| `mise exec -- go version` | Pass | `go version go1.27.1 linux/amd64`. |
| `mise exec -- bun --version` | Pass | `1.4.2`. |
| `mise exec -- golangci-lint version` | Pass | `2.13.2`, built with Go 1.27.0. |
| `mise exec -- bunx playwright install --list` (from `frontend/`) | Pass | Project Playwright 1.63.0 references installed Chromium/headless shell 1243. Browser download unnecessary; subsequent suite passed. |
| `mise run golangci-lint` | Pass | SPA build completed; 46 active linters, zero remaining issues. |
| `mise run test:unit` | Pass (cached test results) | Repository race/coverage task exited successfully. Go reused cached package results; this is not a fresh race execution. API coverage 89.5%, config 89.5%, notification 85.2%; database unit-only coverage 7.0% excludes integration evidence. |
| `mise run frontend:check` | Pass | svelte-check: zero errors and warnings; TypeScript check exited successfully. |
| `mise run frontend:lint` | Pass | ESLint exited successfully. |
| `mise run frontend:format:check` | Pass | All matched files use Prettier style. |
| `mise run frontend:test` | Pass | Vitest: 33 files, 299 tests passed. jsdom emitted `Not implemented: navigation to another Document`; browser navigation remains separately assessed. |
| `mise run frontend:test:e2e` | Interrupted | 23 tests scheduled with four workers; 12 printed successful results before the enclosing 120-second command timeout sent SIGTERM. No full-suite result. API responses are mocked. |
| `mise run frontend:test:e2e` (standalone retry) | Pass | All 23 tests passed in 24.8 seconds with four workers. Mocked API coverage; no snapshot updates. |
| `mise run govulncheck` | Pass with non-reachable module advisory | Zero affected symbols/imported packages. GO-2026-5932 concerns unmaintained `golang.org/x/crypto/openpgp` in required module v0.57.0; this scan reports no imported vulnerable package or call. Not a confirmed exploitable application finding. |
| `mise run frontend:audit` | Pass | No vulnerabilities found across 286 packages. |
| `mise run test:integration` | Pass | Fresh race/coverage execution against PostgreSQL; migrations 1–7 applied. Transfer concurrency/rollback, atomic session rotation, notification snapshot/read counts, commit-gated publication, and cross-replica listener tests passed. Database coverage 79.6%. |
| `docker compose --profile test ps` (before and after integration) | Pass | No services running before invocation or after task-owned teardown. Test containers and network removed. |
| `mise run app:build` | Pass | Production SPA and embedded Go binary built successfully; real-stack assessment below. |
| `mise run compose:test:up` / `mise run compose:test:down` (runtime fixture) | Pass | Fresh local PostgreSQL/Mailpit fixture created and removed. |
| `./dist/simplebank serve` (configuration below) | Pass | Startup migrations, readiness, user journey, and ordered SIGTERM shutdown observed. |
| Local Python Mailpit-link helper | Pass after interpreter correction | `python` was unavailable; identical standard-library helper ran with `python3` and consumed two verification links successfully. No codes retained. |
| `git diff --check`; new-report whitespace check; relative-link review | Pass | No whitespace diagnostics; README/ADR/report link targets exist. Final intended Git changes are this report and its documentation-index link. |
| `openspec validate audit-go-backend-svelte-frontend --strict` | Pass | Report-only change accepted with explicit `skip_specs`. Planning/task files are Git-ignored in this checkout. |

The frontend commands ran in one sequential shell call:

```sh
mise run frontend:check && mise run frontend:lint && mise run frontend:format:check && mise run frontend:test && mise run frontend:test:e2e
```

The timeout covered that entire sequence, not 120 seconds dedicated to Playwright. A standalone retry with a 600-second shell limit passed all 23 tests. No snapshots were updated. Integration and runtime results were subsequently collected as recorded above.

The interrupted run left its Vite server running. Its start time matched this audit's browser invocation; the audit-owned Vite/Bun processes were terminated and a subsequent process check found neither remaining. At that checkpoint, `git status --short` showed only the new `docs/audits/` directory; tracked application files were unchanged. The final deliverable also adds the report link in `docs/README.md`. OpenSpec task updates are present in the planning directory, which does not appear in normal Git status.

## Coverage matrix

Each reviewed area links to source/check evidence below; a reviewed area can still contain findings or explicitly untested conditions.

| Area | Anchors and invariants | Checks/evidence | Status and gaps |
| --- | --- | --- | --- |
| Identity and sessions | API, session transactions, password/token/secret, worker/mail; authentication, digests, rotation, origin/cookies, stale responses | Source trace below; fresh session integration tests; local SMTP verification; browser identity-change probe | Reviewed; mixed-principal frontend state confirmed; logout race remains a hypothesis |
| Accounts and money | Account/transfer handlers, SQL/migrations/transactions, currency, frontend money; ownership, ledger, replay, locking, limits, precision (ADR-0005) | Fresh PostgreSQL safety tests, local ledger comparison, input boundary reproduction | Reviewed; numeric UI precision and combined replay/limit ordering findings |
| Notifications | Notification SQL/API, listener/hub, SSE client/stores; atomic persistence, owner scoping, bounded delivery, reconciliation (ADR-0007) | Fresh integration and mocked E2E, live sender SSE/read-to-history journey | Reviewed; frontend snapshot convergence/order findings; saturation not load-tested |
| Lifecycle and serving | Composition/config, HTTP/SPA, worker/listener/pool; startup/shutdown, deadlines, readiness, fallback/caching | Composition/API tests, real startup/readiness, HTTP probes | Reviewed; bare API fallback finding; runtime TLS/proxy path not exercised |
| Frontend behavior and accessibility | App/pages/controls/stores; session boundaries, forms, retries, focus/keyboard, announcements, themes, responsive layout | Read-only Svelte review, 299 Vitest and 23 mocked Playwright tests, real forms, 320px keyboard/overflow check | Reviewed; empty deposit, recovery UI and refresh-focus findings; no assistive-technology certification |
| Performance | Query/index alignment, lock/pool/stream bounds, request/render work, production assets | SQL/index review, production network observations and authenticated interaction trace | Reviewed; small local dataset only; initial LCP unavailable; no throughput benchmark |
| Architecture and maintainability | Layer ownership, accepted ADRs, duplication, unnecessary flexibility | ADR-aligned source review, CI/release and Docker/Caddy inspection | Reviewed; avoid new repository wrappers; Docker target-architecture configuration finding |
| Tooling and coverage | mise, manifests/locks, CI/release, Docker/Compose/Caddy, test seams | All prescribed automated checks and app build passed | Reviewed; mocks omit important cross-layer cases; module-only advisory not reachable in scan |

## Confirmed findings

### F-01 — Identity-changing refresh retains the previous user's notifications

**Medium · client-side privacy/session isolation · runtime reproduction with one simulated expiry response, independently source-verified.**

- **Locations:** `frontend/src/lib/stores/auth.svelte.ts:145–168`, `frontend/src/App.svelte:25–37`, `frontend/src/lib/stores/notifications.svelte.ts:378–385,506–532`; renewal identity comes from `internal/api/user.go:231–283`.
- **Invariant:** session-owned caches and in-flight work must not survive a change of authenticated principal. `#performRefresh` replaces the user/token without comparing usernames or advancing the generation. App restarts notifications only for a changed generation, and reconciliation retains rows absent from the latest page.
- **Reproduction:** sign in as Alice and retain a sent notification. Perform Bob's login in the same cookie context without reloading Alice's SPA (equivalent to another tab replacing the shared refresh cookie). Return one synthetic 401 for the next `/api/v1/accounts` fetch to model Alice access-token expiry; dispatch `visibilitychange` to trigger refresh. All login/renewal responses and subsequent account requests use the real local backend.
- **Observed:** header changed to `auditbob`, account inventory became Bob's USD 0.25 account, but the notification popover still showed Alice's **Sent −USD 0.25**, with count zero. Bob's durable notification is a separate received row. Expected: clear old-principal state before displaying Bob's session.
- **Limits/counterevidence:** requires a shared browser context with previous authenticated state. Server notification queries remain owner-scoped. No arbitrary remote disclosure, server authorization bypass, or use of a forged token was demonstrated. Explicit logout generation resets work in ordinary tests.
- **Coverage gap:** `auth.svelte.test.ts:442–468,493–542` and `App.test.ts:94–103,199–227,246–259` cover same-user renewal/sign-out, not a different user returned from renewal.
- **Minimal remediation:** make principal replacement an explicit session boundary, invalidating account and notification state and old work. Merely advancing generation is insufficient if accounts reset only in App's signed-out branch. Verify a populated Alice→Bob renewal with real cookie sharing and a deferred old response.

### F-02 — Decimal money loses precision at numeric input and formatting boundaries

**Low · monetary correctness · reproduced in browser, database, and actual money helper.**

- **Locations:** `frontend/src/lib/components/TextField.svelte:61–74`, `pages/NewAccountPage.svelte:89–108,190–201`, `pages/TransferPage.svelte:96–133,247–257`, `frontend/src/lib/money.ts:30–37,61–81`.
- **Invariant/root cause:** valid decimal amounts must retain exact minor units. Numeric Svelte binding coerces the text to a JavaScript number before the integer-string parser sees it; formatting divides exact minor units into an inexact major-unit number.
- **Reproduction:** configure an EUR opening cap of `9007199254740991`; enter `90071992547409.91` into the actual opening form and submit. Query `SELECT currency,balance FROM accounts WHERE owner='auditalice' ORDER BY currency;` in the disposable database.
- **Observed:** EUR balance persisted as `9007199254740990`, not `9007199254740991`. With the actual helper, parsing the raw string yields the expected integer, parsing `Number(raw)` yields one cent less, and formatting `Number.MAX_SAFE_INTEGER` yields `€90,071,992,547,409.90`. `Number('1.0000000000000001')` also becomes `1`, evading excess-precision rejection before parsing.
- **Limits:** the large opening example needs a sufficiently high configured cap; default/demo caps may reject it. Backend and ledger faithfully record the submitted integer; no ledger divergence or cross-principal monetary exploit was established.
- **Coverage gap:** `money.test.ts:4–13,31–75` omits real numeric-binding/max-safe formatting boundaries; page tests use ordinary values.
- **Minimal remediation:** preserve raw decimal strings through amount inputs and format exact minor-unit values without intermediate floating-point major units. Verify actual DOM input at the maximum, excessive precision, USD/EUR cents, and whole-unit VND; retain server-side limits.

Runnable helper reproduction from the repository root:

```sh
mise exec -- bun -e 'import {parseAmountToMinor,formatMoney} from "./frontend/src/lib/money.ts"; const x="90071992547409.91"; console.log({raw:parseAmountToMinor(x,"EUR"),bound:parseAmountToMinor(Number(x),"EUR"),display:formatMoney(Number.MAX_SAFE_INTEGER,"EUR")});'
```

### F-03 — Clearing an optional deposit prevents zero-balance account creation

**Low · form correctness · browser reproduction and installed-version source verification.**

- **Locations:** `frontend/src/lib/pages/NewAccountPage.svelte:23,89–96`, `components/TextField.svelte:61–74`; installed Svelte `src/internal/client/dom/elements/bindings/input.js:287–288`.
- **Trigger:** enter `1` in an unused currency's opening-deposit field, press Ctrl+A then Backspace, and submit. The initial untouched blank works; this edited blank does not.
- **Observed/expected:** the empty field reports “Enter an opening deposit greater than zero, or leave it blank.” Expected: create a zero-balance account. No account is committed on this error.
- **Root cause:** installed Svelte 5.57.0 represents emptied number inputs as `null`. The form checks `String(deposit).trim() !== ''`, so `"null"` enters parsing. One initial tool-based empty fill did not dispatch equivalent keyboard behavior; the finding uses the explicit keyboard reproduction.
- **Coverage gap:** `NewAccountPage.test.ts:289–308` checks entered/preserved amounts, not enter→clear→submit.
- **Minimal remediation:** handle the binding's empty value or retain decimal text as in F-02. Verify both untouched and edited-empty deposits create zero-balance accounts.

### F-04 — Concurrent idempotent replay can fail when the winner reaches a limit

**Low · transfer response correctness · independently verified source schedule; specific combined case not executed.**

- **Locations:** `internal/db/transfer_tx.go:38–85,153–164`, `internal/api/transfer.go:58–81`, `internal/api/errors.go:44–46`.
- **Invariant/root cause:** identical concurrent requests should converge on the committed transfer. Both may miss the initial lookup; after account locks serialize them, the loser checks destination capacity and daily usage before reaching the unique insert. Only a uniqueness error triggers concurrent replay.
- **Concrete schedule:** start two identical amount-60 requests with one source/key, no previous outgoing transfers, and daily cap 100; hold the account lock until both have missed the fast path. Winner commits 60. Loser reads spent=60 and rejects `60 > 100−60` before uniqueness/replay. A destination initially at max-safe−10 with two identical amount-10 requests has the analogous capacity failure.
- **Actual/expected:** source proves one success and one 422 limit error are possible for the same committed logical transfer; expected replay success. No duplicate debit, cap bypass, or permanent replay failure: a later fast-path retry succeeds.
- **Coverage gap:** `transfer_safety_test.go:103–147` forces concurrent same-key misses without a daily limit; its daily-limit test uses different keys. The maximum-destination test is not concurrent.
- **Minimal remediation:** re-check source/key replay after acquiring the serialization locks and before winner-sensitive validation. Preserve parameter-conflict checks and all safety guards. Add combined same-key/limit-boundary cases to the existing PostgreSQL tests.

### F-05 — Notification pages can restore old metadata and keep stale read state

**Low · notification consistency · complete source reasoning, independently verified; no timing-specific runtime reproduction.**

- **Locations:** `frontend/src/lib/stores/notifications.svelte.ts:155–194,345–385`; backend snapshot semantics in `internal/db/notification_tx.go:27–58`.
- **Response-order case:** start load-more at unread count 1, reconcile a new first page at count 2, then complete the old load-more. It checks only session/mutation epochs and unconditionally assigns count/cursor, allowing 2→1 regression. Reconciliation does not advance that mutation epoch.
- **Convergence case:** load more than 20 notifications, mark all read in another tab, then visibility-reconcile. Only first-page rows are replaced; older cached rows remain unread. Paging over their updated server rows discards duplicate IDs rather than refreshing fields. The UI can show unread rows beside a zero count.
- **Expected:** global metadata must not regress behind a newer snapshot, and fetched authoritative read state must update existing rows. Database durability, ownership, and per-request repeatable-read consistency remain intact.
- **Coverage gap:** `notifications.svelte.test.ts:279–301,336–377` covers duplicate suppression and responses crossing local mutations, not newer reconciliation or changed fields on duplicates. E2E pagination is sequential.
- **Minimal remediation:** order snapshot metadata application and merge updated fields for known rows, with a clear refresh strategy for retained history. Verify reversed pagination/reconciliation completion and cross-tab mark-all over multiple pages. Do not weaken mutation-epoch protection.

### F-06 — Verification failure instructions lead to a sign-in dead end

**Low · onboarding/recovery usability · independently verified source flow.**

- **Locations:** `frontend/src/lib/pages/VerifyEmailPage.svelte:98–109`, `LoginPage.svelte:63–83,86–135`, `internal/api/user.go:154–178,341–366`.
- **Trigger:** an unverified user visits an expired link and follows the page instruction to sign in and request another email.
- **Actual/expected:** login rejects unverified accounts, and the SPA exposes no resend control. The public backend resend endpoint works independently of authentication. Expected: a reachable recovery action or accurate instructions; this is not permanent backend account lockout.
- **Coverage gap:** `VerifyEmailPage.test.ts:46–62` asserts the login link without following the recovery journey; backend tests correctly expect unverified rejection and generic resend acceptance.
- **Minimal remediation:** expose the existing email-based resend action from verification/login recovery, retaining generic responses/rate-limit handling, and correct the instructions. Verify expired-link→resend→new-link verification without signing in.

### F-07 — Bare API prefix falls through to HTML

**Low · API routing contract · runtime curl/browser reproduction and source verification.**

- **Location:** `internal/api/spa.go:29–46` (`serveSPA`).
- **Trigger/evidence:** `curl -sS -o /dev/null -w '%{http_code} %{content_type}\n' http://127.0.0.1:18080/api` returned `200 text/html; charset=utf-8`; the same probe for `/api/missing` returned `404 application/json`.
- **Root cause:** cleaned `api` does not match `strings.HasPrefix(name, "api/")`. Cleaning `/api/` also produces `api`. Expected: API-prefix paths remain JSON errors, per the repository routing boundary.
- **Impact/limits:** inconsistent client response type/status; no demonstrated sensitive disclosure or authorization bypass.
- **Coverage gap:** `internal/api/spa_test.go:83–103` tests only `/api/unknown`.
- **Minimal remediation:** match exact `api` as well as descendants; add `/api` and `/api/` boundary tests while preserving client-route fallback.

### F-08 — Docker's target architecture is not declared in the build stage

**Low · release packaging · independently verified source configuration; published binaries not inspected.**

- **Locations:** `Dockerfile:2,16,48–57`, `mise.toml:20–34`, `.github/workflows/ci.yml:218–234`, `.github/workflows/release.yml:117–129`.
- **Trigger/root cause:** the builder runs on `$BUILDPLATFORM`, but `RUN ... GOARCH=$TARGETARCH` uses an automatic BuildKit argument not redeclared with `ARG TARGETARCH` inside that stage. `TARGETOS` and `BUILDARCH` are declared; `TARGETARCH` is absent. An empty GOARCH defaults the Go build to the builder architecture.
- **Expected/possible outcome:** each image must contain its target executable; an amd64 builder can instead place amd64 code in the arm64 image. Same-architecture builds still work. This run did not inspect a released image or demonstrate a production outage.
- **Coverage gap:** local app build covers host architecture; CI lists/inspects image metadata rather than checking each executable's architecture or booting both targets.
- **Minimal remediation:** declare `ARG TARGETARCH` in the builder stage. Verify extracted executable headers and startup for both requested platforms in a disposable build; do not infer correctness from manifest labels alone.

### F-09 — Background reconciliation removes the focused transfer form

**Low · keyboard accessibility · source-confirmed reachable lifecycle, specialist-reviewed.**

- **Locations:** `frontend/src/lib/stores/notifications.svelte.ts:404–406` → `accounts.svelte.ts:23–37` → `frontend/src/lib/pages/TransferPage.svelte:193–216`.
- **Trigger:** focus the recipient or amount control while a live/visibility/reconnect reconciliation starts an account load and remains pending across a render.
- **Actual/expected:** `accounts.loading=true` replaces the entire form with loading UI even when cached accounts exist. The focused DOM input is removed and recreated; no same-route restoration exists. Expected: background refresh retains usable controls/focus. Bound values survive; data loss and a specific browser fallback-focus target are not claimed.
- **Coverage gap:** the E2E toast non-interference case does not exercise a focused transfer control disappearing on account refresh.
- **Minimal remediation:** distinguish initial loading from background refresh and retain the form when usable cached inventory exists. Verify focus/value retention through a delayed notification-triggered load and error/retry.

## Source-review evidence

### Identity, sessions, and email (task 3.1)

Traced `internal/api/user.go`, `middleware.go`, `session_cookie.go`, `validator.go`, `internal/token/{maker,jwt_maker}.go`, `internal/secret/secret.go`, `internal/mail/smtp.go`, `internal/worker/{client,verify_email}.go`, `internal/db/{user,session,verify_email}_tx.go`, and `query/verify_emails.sql`.

Registration validates a 15-character/72-byte password, hashes before uniqueness handling, responds generically, and enqueues verification inside the user transaction. Unknown-user login performs bcrypt comparison. Token verification restricts algorithms and token types; refresh tokens carry a nonce while retaining the stable session ID. Rotation locks and checks username/digest/block/expiry before replacing the digest. Logout blocks that stable ID and clears cookies on failure. Verification consumes an unused unexpired digest and verifies the user in one transaction. Email HTML escapes the full name; SMTP requires TLS unless explicitly configured insecure and refuses plaintext credentials. Origin/Fetch Metadata guards apply to cookie-consuming endpoints; access endpoints require bearer credentials. Logs use the URL path rather than verification query parameters.

Evidence: passing API credential/cookie/origin tests, `TestRotateSessionTx_ConcurrentReuse`, `TestRotateSessionTx_LogoutInterleavingsStableID`, `TestCreateUserTxRollbackOnAfterCreateError`, and `TestVerifyEmailTx`; live local registration produced two SMTP messages whose verification links returned 200/verified. Browser identity-transition candidates remain separately tracked; the backend checks do not establish browser-cache isolation.

### Money and accounts (task 3.2)

Traced `internal/api/{account,transfer,meta}.go`, `internal/db/{account,transfer}_tx.go`, `query/{accounts,transfers}.sql`, and migrations 1–7. Ownership is checked before destination lookup and before exposing account/history data. Positive, bounded minor units are enforced at HTTP/SQL boundaries; opening deposits post matching ledger entries. Transfer rows, entries, balance updates, and notifications share a transaction. Deterministic UUID lock order avoids opposing-transfer deadlocks; currency and destination capacity are revalidated on locked rows. Rolling totals use `clock_timestamp()` and numeric SUM clamped to the supported limit. Source-scoped keys reject changed immutable parameters and replay without a second debit. Recipient snapshots are excluded from the HTTP result.

Evidence: fresh integration tests listed in the ledger cover limits, overdrafts, safe integer bounds, rollback, lock contention, ledger rows, same-key concurrency, and replay conflicts. The real UI transfer moved 25 USD minor units from Alice to Bob, leaving 75 and 25 respectively, and produced sender activity plus a live notification. Numeric input/display defects are frontend boundary findings, not a failure of the database's integer representation.

### Notifications (task 3.3)

Traced `internal/api/notification.go`, `internal/notification/{listener,hub}.go`, `internal/db/notification_tx.go`, `query/notifications.sql`, and notification schema. Persistence and NOTIFY are commit-gated; SQL derives ownership/amount/balance from the transfer and account. REST lists use an owner filter and stable `(created_at,id)` cursor; count/list share repeatable-read state. Read mutations use owner-qualified updates and owner advisory locks. Hub queues hold 16 IDs and drop excess invalidations rather than block publishers. Listener reconnection backs off from 100 ms to 5 s. SSE binds to bearer identity and expiry, unsubscribes on exit, and refreshes write deadlines; a 15-second comment keepalive does not itself reconcile durable data.

Evidence: passing notification owner/cursor/read-state tests, `TestNotificationPublishIsCommitGated`, `TestListNotificationsPageUsesOneRepeatableReadSnapshot`, `TestNotificationReadTransactionsSerializeCountsPerOwner`, `TestListenerCrossReplicaDelivery`, hub concurrency/backpressure and listener cancellation tests. Live sender notification incremented the badge, announced the transfer, and marking it read navigated to account history. Browser response ordering and older-row convergence require separate review below.

### Composition, serving, configuration (task 3.4)

Traced `cmd/app/main.go`, `internal/db/migrate.go`, `internal/worker/client.go`, config validation, API middleware and SPA serving. Startup applies domain migrations under a session advisory lock, migrates River, starts listener then River then HTTP. Partial build errors close the pool; shutdown drains HTTP, stops River with bounded cancellation fallback, stops the listener, then closes the pool. TLS cert/key pairing and secure-cookie consistency are validated. Readiness is a bounded pool ping. HTTP deadlines are bounded; SSE skips the general 30-second context timeout but keeps per-frame write deadlines and token expiry.

Existing composition and API tests passed. The built binary started against the disposable stack, applied domain/River migrations, and returned readiness 200. SPA content-hashed assets have immutable cache headers; shell routes revalidate. Bare `/api` was observed returning HTML 200 and is a confirmed fallback-boundary defect to document. Direct/proxy TLS configurations were inspected; their deployed behavior was not exercised in this run.

### Frontend state, forms, and accessibility (tasks 4.1–4.3)

A read-only Svelte specialist traced App/Root, auth/account/notification stores, API client/response validation/SSE, router, Login/Register/VerifyEmail, Transfer/NewAccount/AccountHistory, shared controls, and themes. A separate read-only verifier checked candidate root causes and counterevidence. No test or component was modified. Svelte MCP analysis reported no `issues`; general effect/lifecycle suggestions were treated as advice, not defects.

Confirmed controls include single-flight refresh, one authenticated retry, classified errors, account reset generations, newest-load-wins, route/auth guards for activity loads, normalized same-instance transfer keys, notification reconciliation coalescing, abort/timer/visibility cleanup, labels/error associations, route announcements, keyboard controls, focus-visible styles, forced-colors and reduced-motion CSS. Relevant tests: `client.test.ts:189–435`, `auth.svelte.test.ts:268–341,373–542`, `accounts.svelte.test.ts:48–188`, `notifications.svelte.test.ts:102–492`, `TransferPage.test.ts:321–614,663–691`, and `AccountHistoryPage.test.ts:105–253`.

The passing browser suite covers themes/reflow, initial validation focus, navigation history, notification UI, and toast non-interference. Real runtime observations confirmed route focus on `main`, Tab reaching the activity Back link, and document width 320 at a 320×800 emulated viewport. That does not cover a live refresh removing a focused form, multi-page read-state convergence, or identity-changing renewal. Those are findings below. Svelte behavior references: [effects](https://svelte.dev/docs/svelte/$effect), [lifecycle](https://svelte.dev/docs/svelte/lifecycle-hooks), and [numeric bindings](https://svelte.dev/docs/svelte/bind#input-bind:value). For the empty-input case, the installed Svelte 5.57.0 implementation returns `null`; the public binding documentation's `undefined` description is not used as version-specific evidence.

### Architecture, maintainability, and supply chain (task 4.4)

The wide Store, SQL source/generated boundary, transaction ownership, domain-specific credential/currency packages, and composition-root lifecycle fit accepted ADRs. No concrete benefit justifies adding narrow per-handler repositories or a service supervisor. The consequential complexity is asynchronous ordering across session, account, and notification stores: each guard protects one class of stale work, but different-principal refresh and independent snapshot responses bypass those assumptions.

Inspected `mise.toml`, manifests/locks, `.github/workflows/{ci,release}.yml`, Dockerfile, Compose, and Caddy. CI actions are SHA-pinned with read-only top-level permissions and job timeouts; release privileges are job-specific. CI includes lint, security, frontend, real PostgreSQL, and HTTP/TLS liveness smoke checks. Production image runs distroless/nonroot; mise download checksums are verified. Compose endpoints bind loopback and use explicitly documented demo credentials. Caddy terminates TLS; the app trusts the configured proxy subnet. Release signing/provenance/SBOM limitations remain as documented, not new findings. Mutable dev image tags affect reproducibility but are not evidence of compromise. The Docker target-architecture omission merits a separate packaging fix and per-platform verification.

## Hypotheses requiring validation

These remain hypotheses without severity; they are not included in the nine confirmed findings:

1. **Late transfer completion can initiate refresh during logout.** `TransferPage.svelte:122–141` unconditionally calls `accounts.load()` after await; `auth.svelte.ts:126–168` permits refresh while `loggingOut`; logout completion at `:194–207` does not clear auth again. A new-generation refresh might restore local auth before server revocation wins. Validate with deferred transfer/logout/renew responses; the existing stale-pre-logout-refresh test does not prove this schedule.
2. **Overlapping inventory loads can skip page initialization.** Latest-load-wins may discard the page's own request while a notification refresh is pending; the page's continuation skips source/currency initialization and is not rerun when the newer load completes. Check `TransferPage.svelte:49–58`, `NewAccountPage.svelte:36–45`, and actual stores with reversed response ordering.
3. **Repeated identical transfer validation may not refocus.** Clearing/reassigning the same error in one synchronous submit can leave `TextField.svelte:51–55` with no error transition. Verify a second unchanged invalid submit; Login/Register explicitly focus each time.
4. **Transport gaps can delay live recovery.** Hub overflow drops invalidations and listener reconnect does not signal connected browsers to reconcile. Durable REST history remains correct; quantify recovery latency after a LISTEN interruption before assigning impact. Browser reconnect/visibility and token expiry provide later recovery, so permanent notification loss is not claimed.

Other observable ceilings: activity UI requests only the first 50 transfers; transfer idempotency intent is component-local and is lost on remount. Neither is labeled a requirement violation without a clarified recovery/history expectation. A late Set-Cookie response cannot be neutralized merely by discarding its JSON; cookie-aware login/logout race testing remains useful.

## Real-stack and performance evidence

The runtime used `mise run app:build`, a fresh `mise run compose:test:up` stack, and `./dist/simplebank serve` with stdout/stderr redirected to `/tmp/opencode/simplebank-audit-server.log`. Launch configuration: `HTTP_ADDR=127.0.0.1:18080`, test PostgreSQL on loopback port 5433 (`DB_SOURCE` credentials omitted), dummy `JWT_SECRET` of at least 32 characters, `SMTP_FROM=audit@example.test`, `SMTP_HOST=127.0.0.1`, `SMTP_PORT=1026`, `SMTP_INSECURE=true`, `SESSION_COOKIE_SECURE=false`, `PUBLIC_BASE_URL=http://127.0.0.1:18080`, and `ACCOUNT_OPENING_LIMITS='{"USD":9007199254740991,"EUR":9007199254740991,"VND":9007199254740991}'`. Transfer limits were unset. The elevated demo opening caps were intentional boundary fixtures, not production recommendations. Browser credentials and verification codes are not retained here.

Observed journey with disposable Alice/Bob identities:

1. Browser fetch registration returned 202 for both users. River delivered two messages to Mailpit; following their links through the verification API returned 200 and `is_verified: true`. This verifies email delivery/API consumption, not every SPA verification-page state.
2. Bob logged in through the API and opened an empty USD recipient account. Alice logged in through the actual UI, opened USD and EUR accounts, and submitted a USD 0.25 transfer through the form.
3. The transfer receipt showed USD 0.75 remaining. The live notification badge became one and announced a sent transfer. Activating the notification marked it read and navigated to activity showing −USD 0.25.
4. A SQL join of accounts and entries showed Alice USD balance/ledger 75/75, Bob USD 25/25, and Alice EUR 9007199254740990/9007199254740990. This confirms ledger consistency for the fixture while exposing input rounding.
5. UI logout returned to login and removed the authenticated header. A separate controlled identity-transition probe is detailed under F-01.
6. SIGTERM produced ordered logs: HTTP shut down → worker shutting down → notification listener shutting down → database pool closed. `mise run compose:test:down` removed the audit-owned test services/network; a final Compose `ps` was empty. The isolated audit browser tab was closed.

HTTP probes observed shell `Cache-Control: no-cache`, API policy `no-store`, hashed JS `public, max-age=31536000, immutable`, and CSP on all inspected responses. Bare `/api` returned HTML 200; `/api/missing` returned JSON 404. The direct server did not return content encoding for the JS asset. No production TLS behavior is inferred from this local HTTP fixture.

### Performance sample and limits (tasks 5.2–5.3)

- Linux x86_64, Chrome 153, CPU 1× and no network throttling; production assets from the embedded Go server on loopback. Dataset: two users, three accounts, one transfer, two durable notifications. This is not representative load.
- Initial login reload trace reported CLS 0.00 but no usable LCP. A later cache-bypassed 1440×900 reload yielded five resource entries excluding the document: JS 133,942 encoded bytes, CSS 109,193, Latin font 45,712, favicon 9,522, and an empty renew response. JS/CSS/font transfers were uncompressed. Do not interpret absent paint entries or a delayed first-paint sample as a site timing result.
- A recorded authenticated Dashboard → Transfer interaction at 1440×900 yielded trace-reported LCP 111 ms for the SPA navigation, interaction latency 112 ms, CLS 0.00. These are one local interaction's observations, not field INP or an SLA.
- UI route focus and a keyboard Tab were checked on account activity; at emulated 320×800, document/viewport widths were both 320. Mocked Playwright supplies broader responsive/theme/axe coverage. Screen readers and full keyboard traversal were not exercised manually.
- SQL review: notification history has `(owner,created_at DESC,id DESC)` and unread-owner partial indexes. Transfers have source/destination indexes and `(from_account_id,created_at)` for source/time access. The rolling query uses volatile `clock_timestamp()`; no EXPLAIN or representative-volume test establishes how much of the time predicate becomes an index condition. Transfer history combines source/destination with OR, sorting and OFFSET; deep history may cost more but no measured bottleneck is claimed. Accounts are unique per owner/currency (three supported currencies), so current account lists are naturally small.
- Streams retain one 16-ID queue per subscriber; listener uses a dedicated connection, separate from the pool. The subscriber count itself is not bounded by this queue size. Global hub locking and notification unread-count scans deserve load evidence before a redesign. Existing concurrency tests prove selected invariants, not throughput.
- One completed transfer generated two account-list refreshes (submit completion and notification reconciliation). This is observed duplicate work on tiny responses, not a demonstrated slow path. The stronger issue is that such background refresh removes a focused transfer form (F-09).
- Compression could reduce JS+CSS from about 243 KB to the build-reported 61 KB gzip combined on first load. This is a payload opportunity; no measured user-visible latency saving is claimed. Prefer server/proxy compression if real network measurements justify it.

## Known limitations and accepted trade-offs

The [security guide](../security.md) already documents per-process rate limits, access JWT validity until expiry after logout, migration/runtime credential sharing, unsigned container releases and missing attestations/SBOM, and absence of MFA/password reset/account recovery. These are not new findings. Demo opening balances are an explicit fixture affordance, not a production funding mechanism. ADR-0001 accepts the wide Store; ADR-0007 makes durable REST state authoritative while NOTIFY/SSE accelerates updates, and keeps readiness as a pool ping.

## Coverage gaps

All planned areas have source/check dispositions above. Limitations remain:

- Initial standalone unit results used Go's cache; the subsequent integration-tag race run executed fresh package tests and PostgreSQL cases. No exhaustive scheduler exploration was performed.
- Playwright's 23 passing cases mock the API. The separate real-stack journey covers the stated local flows, not every browser/API interleaving. F-01 used one synthetic 401 rather than waiting 15 minutes for token expiry.
- Source-only schedules in F-04/F-05/F-09 need targeted regression reproductions during remediation. F-08 needs per-platform artifact/runtime validation.
- Performance uses tiny local data, no network/CPU throttle, one authenticated interaction, and no useful initial-load LCP. No load benchmark, EXPLAIN-at-scale, field INP, or screen-reader certification is claimed.
- Runtime direct TLS/reverse proxy, deployed gateway policies, and production identities/infrastructure were not tested. Configuration and existing TLS unit tests are the evidence available here.
- The initial browser timeout and missing `python` alias were resolved execution issues, not application findings. No application fixes, generated-code edits, dependency updates, or test/snapshot changes were made.

## Prioritized remediation backlog

1. **F-01:** establish a principal-change boundary across auth/accounts/notifications; prove a populated cross-tab identity transition clears old data.
2. **F-02/F-03:** preserve exact decimal input and explicit emptiness; verify maximum-safe input/display and cleared optional deposits.
3. **F-04:** re-check replay under account serialization before dynamic limits; verify concurrent same-key daily/capacity boundaries.
4. **F-05/F-09:** correct notification snapshot ordering/row refresh and retain focused forms during background loads; verify deferred responses and keyboard focus.
5. **F-06/F-07:** use the existing resend API for recovery and close the bare API fallback edge; add journey/boundary tests.
6. **F-08:** declare the Docker target architecture and verify each image's actual executable before release.
7. Resolve the named hypotheses with focused disposable checks, and collect representative performance evidence before adding caches, indexes, services, or broad refactors.

These are recommended follow-up changes, not fixes authorized or implemented by this audit.

## Remediation progress — 2026-09-20

The separately authorized `fix-backend-frontend-audit-findings` change is implemented and verified with the explicit local ARM64 runtime waiver below. The original audit above remains a record of the pre-fix revision.

- F-01/F-02/F-03/F-05/F-06/F-09: frontend changes and regressions implemented. Frontend check/lint/format gates passed, with 339 unit tests and 30 browser tests. Real PostgreSQL/Mailpit browser verification passed as detailed below.
- F-04: replay is rechecked through transaction-local queries after account locks, before mutable limits. Full integration tests passed, including same-key unlimited/daily/destination-boundary cases with a two-connection pool, exactly one movement, two notifications and no duplicate publication. Existing conflict, distinct-key limit and deadlock cases passed.
- F-07: `/api` and `/api/` return 404 JSON. Per the user's explicit Option 1 decision, unauthenticated unknown `/api/v1/*` retains the existing 401 JSON authentication precedence. Focused SPA/API tests passed.
- Backend `mise run golangci-lint:fmt`, `mise run golangci-lint`, `mise run test:unit`, `mise run test:integration`, `mise run govulncheck`, and `mise run app:build` passed. No SQL inputs or generated query output changed. The previously noted module-only OpenPGP advisory remains unreachable according to the scan.
- F-08: Docker now declares `TARGETARCH`. `mise run docker:build` built both targets; `mise run docker:verify` inspected matching x86-64 and AArch64 ELF headers and `nonroot:nonroot` runtime users. amd64 `version` startup passed. ARM64 startup failed with `exec format error` because the local host lacks ARM64 runtime/emulation. The user explicitly requested skipping the remaining local ARM64 verification: **ARM64 runtime is skipped, not passed**. CI retains pinned QEMU and both target checks; CI has not been executed here. Script syntax and ShellCheck passed, and a mocked-command negative check verified an x86-64 executable is rejected when checking arm64. The new verifier is explicitly allowlisted in `.gitignore` so CI receives it.
- The verifier removed its temporary containers and extracted files on failure. Built images remain locally available for the outstanding startup check. No images were published.

### Real-stack remediation checks

Used the rebuilt embedded binary (`mise run app:build`) on loopback `127.0.0.1:18080` with disposable `mise run compose:test:up` PostgreSQL/Mailpit services, the local-only configuration described in the original audit, fresh fixture identities, and maximum-safe opening caps. No production data was used.

- **F-02:** entering `90071992547409.91` in the actual USD opening form returned 201 with exactly `9007199254740991` minor units and displayed `$90,071,992,547,409.91`. A subsequent 25-unit transfer left `9007199254740966` in PostgreSQL, with 25 in the recipient account.
- **F-03:** selecting EUR, entering `1`, then Ctrl+A/Backspace and submitting returned 201 with balance zero; PostgreSQL confirmed EUR zero.
- **F-06:** a fixture verification record was expired in the local DB and its completed River deduplication key cleared to model eligibility for a replacement without waiting through the real deduplication period. The expired link showed failure; the public form returned generic accepted feedback, Mailpit received a replacement, and the SPA verified it without login. The query credentials were removed from the address bar. All three fixture users ended verified. Deduplication policy itself was not changed or bypassed in application code.
- **F-01:** Alice held a sent notification and recipient selection; Bob's real login replaced the shared browser refresh cookie. A single injected notification 401 modeled Alice access-token expiry. Real backend renewal switched the header to Bob, cleared the recipient input, removed Alice's sent notification, and displayed Bob's received notification. No mock login/renewal/account data was used.
- **F-07:** direct requests returned `/api` → 404 JSON, unauthenticated `/api/v1/unknown` → 401 JSON, and `/transfer` → 200 HTML as agreed.
- Browser-harness issues (an unavailable server-side `URL` global and a route already handled during interception setup) were corrected and the affected checks completed successfully; these are not application failures.
- The already-recorded overlapping account-load initialization hypothesis was observed: after reopening account creation, the available radios were EUR/VND while the deposit label initially remained USD. Explicitly selecting EUR allowed the blank-deposit check. This pre-existing hypothesis is outside the nine confirmed findings and remains follow-up work; the remediation does not claim to fix it.

The verification server was terminated, the test services/network removed with `mise run compose:test:down`, and the browser closed. Final diff review preserved existing audit work and found no SQL/generated-output, dependency, API-payload, or schema changes. Local ARM64 runtime and the named pre-existing audit hypotheses remain the disclosed limitations.
