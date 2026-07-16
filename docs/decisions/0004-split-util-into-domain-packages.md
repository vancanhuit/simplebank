# ADR-0004: Split `util` into domain packages, separating crypto from non-crypto randomness

## Status
Accepted

## Date
2026-07-16

## Context
`internal/util` had grown into a catch-all: password hashing, currency
constants and validation, a crypto-secure token generator, and a non-crypto
random string/owner generator all lived in one package named after no domain.
A caller reading `util.RandomString` and `util.Secure`-style helpers side by
side could not tell from the package name which functions are safe for
security-sensitive use.

Two forces made this worth fixing rather than leaving alone:

- **Naming.** `util` names a location, not a concept. Every call site read
  `util.X`, which tells a reader nothing about the boundary being crossed.
- **Safety.** The package mixed a CSPRNG-backed secret generator with a
  `math/rand/v2`-backed generator meant only for test fixtures and display
  names. Housing both under one name invites a caller to reach for the
  predictable generator where an unpredictable one is required — a silent
  security defect with no compile-time signal.

## Decision
Replace `internal/util` with four focused packages, each named for its domain
and exporting stutter-free names:

- `internal/password` — `Hash`, `Check` (bcrypt).
- `internal/currency` — `USD`/`EUR`/`VND`, `IsSupported`.
- `internal/secret` — `Token` (`crypto/rand`, hex-encoded). Crypto-secure.
- `internal/random` — `String`, `Owner` (`math/rand/v2`). **Not** crypto-secure;
  for test fixtures and non-sensitive values only.

The `secret`/`random` split is the load-bearing part: the package name now
encodes the security guarantee. `secret.Token` is unpredictable and safe for
verification codes and similar; `random.String` is fast, seedable, and must
never back a secret.

## Alternatives Considered

### Keep a single `util` package, fix only the naming of functions
- Pros: no import churn; one package to find helpers in.
- Cons: leaves the crypto/non-crypto generators adjacent under a name that
  advertises neither guarantee. The dangerous adjacency — the actual risk —
  survives.
- Rejected: it addresses the cosmetic complaint and skips the safety one.

### Split by kind but keep one `rand` package with two functions
- Pros: fewer packages.
- Cons: a package named `rand` holding both a CSPRNG token and a `math/rand`
  string re-creates the exact ambiguity, just one level down. The name still
  fails to distinguish the guarantee.
- Rejected: the whole point is that the package boundary should carry the
  crypto/non-crypto distinction.

### Merge everything into the packages that use it (no shared packages)
- Pros: no cross-package helper imports.
- Cons: `password.Hash`, `IsSupported`, and secret/random generation are used
  from multiple packages (`api`, `worker`, and several `db` tests). Inlining
  would duplicate them.
- Rejected: these are genuinely shared, single-responsibility helpers.

## Consequences
- Every call site now reads `password.Hash`, `currency.IsSupported`,
  `secret.Token`, or `random.String`, and the package name states the domain
  and — for the two generators — the security guarantee.
- A reviewer seeing `random.` in security-sensitive code has a name-level
  signal that it is wrong; `secret.` is the only correct choice there.
- Cost is import churn: `api`, `worker`, and three `db` test files were
  repointed. This was a one-time mechanical change with no behavior difference.
- More packages exist, but each has one responsibility and a name that earns
  its place. The deletion test passes: removing any of the four would force its
  callers to re-implement real behavior, not merely drop a pass-through.
