# Task 7 Report: Pin Container and CI Supply Chain

## Status
- Completed
- Commit: `8f09c699d1d475b1a96135419ca2fda1a2dd1f83` (`build: pin supply chain inputs`)
- No checksum or image/action digest mismatches detected.

## Files Changed
- `Dockerfile`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `.github/actions/go-cache/action.yml`
- `scripts/check-supply-chain.sh`
- `mise.toml`

## RED / GREEN Evidence

### RED 1: supply-chain checker failed before pinning
Command:
```bash
chmod +x scripts/check-supply-chain.sh && mise run supply-chain:check
```
Output:
```text
[supply-chain:check] $ scripts/check-supply-chain.sh
mutable GitHub Action references:
.github/workflows/ci.yml:23:      - uses: actions/checkout@v7
.github/workflows/ci.yml:24:      - uses: jdx/mise-action@v4
...
.github/workflows/release.yml:145:      - uses: actions/download-artifact@v8
.github/actions/go-cache/action.yml:20:    - uses: actions/cache@v6
.github/actions/go-cache/action.yml:31:    - uses: actions/cache@v6
.github/actions/go-cache/action.yml:42:      uses: actions/cache@v6
[supply-chain:check] ERROR task failed
```
Result: exit 1, expected fail-closed behavior confirmed.

### RED 2: exact multi-platform build exposed arm64 cache pollution
Command:
```bash
source env.bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg BUILD_DATE="$BUILD_DATE" \
  .
```
Output excerpt:
```text
#25 6.147 error: Fail extracting tarball for "@rolldown/binding-linux-x64-musl"
#25 7.541 error: Fail extracting tarball from @rolldown/binding-linux-x64-musl
ERROR: failed to build: failed to solve: process "/bin/bash -o pipefail -c GOOS=$TARGETOS GOARCH=$TARGETARCH ... mise run app:build" did not complete successfully: exit code: 1
```
Root cause: shared Bun install cache mount crossed amd64/arm64 builds; arm64 stage tried consuming x64 artifact.

### GREEN 1: supply-chain checker passed after pinning
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && mise run supply-chain:check
```
Output:
```text
[supply-chain:check] $ scripts/check-supply-chain.sh
```
Result: exit 0.

### GREEN 2: exact multi-platform build passed after arch-scoped Bun cache
Dockerfile fix:
```dockerfile
--mount=type=cache,id=bun-install-${TARGETARCH},target=/root/.bun/install/cache
```
Command:
```bash
source env.bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg BUILD_DATE="$BUILD_DATE" \
  .
```
Output excerpt:
```text
#25 8.636 287 packages installed [8.51s]
#25 45.54 Finished in 45.43s
#26 8.111 287 packages installed [7.98s]
#26 44.59 Finished in 44.47s
WARNING: No output specified with docker-container driver. Build result will only remain in the build cache.
```
Result: exit 0.

## Exact Verification Commands and Output

### Post-commit verification suite
Command:
```bash
mise run supply-chain:check && \
bash -n scripts/check-supply-chain.sh && \
/usr/bin/python3 - <<'PY'
import tomllib, yaml
for path in ['mise.toml']:
    with open(path, 'rb') as f:
        tomllib.load(f)
    print(f'TOML OK {path}')
for path in ['.github/workflows/ci.yml', '.github/workflows/release.yml', '.github/actions/go-cache/action.yml']:
    with open(path, 'r', encoding='utf-8') as f:
        yaml.safe_load(f)
    print(f'YAML OK {path}')
PY
source env.bash && \
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION="$VERSION" \
  --build-arg COMMIT="$COMMIT" \
  --build-arg BUILD_DATE="$BUILD_DATE" \
  . && \
mise run app:build && \
dist/simplebank version && \
mise run govulncheck
```
Output excerpt:
```text
[supply-chain:check] $ scripts/check-supply-chain.sh
TOML OK mise.toml
YAML OK .github/workflows/ci.yml
YAML OK .github/workflows/release.yml
YAML OK .github/actions/go-cache/action.yml
#22 CACHED
#25 CACHED
#27 CACHED
version:    v0.5.1
commit:     da2c5e5e0018eb9be00e144846a12519f53193dc
build date: 2026-08-20T12:05:33Z
Your code is affected by 0 vulnerabilities.
This scan also found 0 vulnerabilities in packages you import and 1
vulnerability in modules you require, but your code doesn't appear to call these
vulnerabilities.
```
Result: exit 0.

### Additional self-review commands
Command:
```bash
git --no-pager diff --stat
git --no-pager diff --check
rg -n 'uses: .*@v[0-9]+' .github || true
rg -n 'uses: .*@latest\b' .github || true
rg -n 'curl https://mise\.run \| sh' Dockerfile || true
rg -n '^FROM ' Dockerfile
```
Output excerpt:
```text
.github/actions/go-cache/action.yml |  6 ++--
.github/workflows/ci.yml            | 67 ++++++++++++++++++++-----------------
.github/workflows/release.yml       | 24 ++++++-------
Dockerfile                          | 24 ++++++++++++--
mise.toml                           |  4 ++++
-- mutable action refs --
-- mutable installer --
-- Dockerfile FROM lines --
2:FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder
63:FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
```
Result: no leftover mutable refs or unpinned base images found.

## Implementation Summary
- Added fail-closed `scripts/check-supply-chain.sh` with comment-compatible immutable-ref matching.
- Added `mise` task `supply-chain:check`.
- Replaced Dockerfile mise curl-pipe installer with versioned binary download + exact SHA-256 verification.
- Pinned both Dockerfile base images to exact digests from brief.
- Replaced mutable GitHub Action tags with exact commit SHAs from brief in CI, release, and composite action files.
- Added dedicated `supply-chain` CI job; Docker job now depends on it.
- Kept local composite action refs unchanged.
- Scoped Bun install cache by `${TARGETARCH}` so exact required multi-arch build passes.

## Self-Review
- Verified all non-local GitHub Action refs use 40-char SHAs with version comments preserved.
- Verified checker remains fail-closed and still allows local composite refs + trailing version comments.
- Verified both `FROM` lines are digest pinned.
- Verified exact multi-arch build succeeds after root-cause fix.
- Verified app build still produces runnable binary metadata output.

## Concerns
- `scripts/check-supply-chain.sh` lives under ignored `scripts/`; required `git add -f scripts/check-supply-chain.sh` during commit.
- `govulncheck` reports module-level advisory `GO-2026-5932` in `golang.org/x/crypto`, but code-path analysis says project is affected by 0 vulnerabilities because vulnerable package is not called.

## 2026-08-21 Follow-up: round 1/5 checker hardening

### Summary
- Replaced the checker's hard-coded file list with discovered `.github/workflows/*.{yml,yaml}` and `.github/actions/**/action.{yml,yaml}` inputs.
- Added explicit failures for mutable `version: latest`, generic `mise.run | sh/bash`, and checksum verification not bound to `/usr/local/bin/mise`.
- Pinned both `docker/setup-compose-action` call sites in `.github/workflows/ci.yml` to `v5.5.0`.
- Added a read-only `supply-chain` job to `.github/workflows/release.yml` and made `docker` depend on it.

### Verification commands and results

#### PASS: checker against current tree
Command:
```bash
mise run supply-chain:check
```
Output:
```text
[supply-chain:check] $ scripts/check-supply-chain.sh
```
Result: exit 0.

#### PASS: shell, YAML, TOML syntax
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && bash -n scripts/check-supply-chain.sh && python3 - <<'PY'
import tomllib, yaml
with open('mise.toml', 'rb') as f:
    tomllib.load(f)
print('TOML OK mise.toml')
for path in ['.github/workflows/ci.yml', '.github/workflows/release.yml']:
    with open(path, 'r', encoding='utf-8') as f:
        yaml.safe_load(f)
    print(f'YAML OK {path}')
print('Shell OK scripts/check-supply-chain.sh')
PY
```
Output:
```text
TOML OK mise.toml
YAML OK .github/workflows/ci.yml
YAML OK .github/workflows/release.yml
Shell OK scripts/check-supply-chain.sh
```
Result: exit 0.

#### RED: synthetic mutable workflow discovered dynamically
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && fixture=.github/workflows/__supply-chain-regression.yml && trap 'rm -f "$fixture"' EXIT && cat > "$fixture" <<'EOF'
name: regression
on: workflow_dispatch
jobs:
  mutable:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: docker/setup-compose-action@4eb059ff7f16592f9c84d5ca339c53cb7c5064e2 # v2
        with:
          version: latest
EOF
if output="$(bash scripts/check-supply-chain.sh 2>&1)"; then
  echo 'unexpected checker pass for synthetic mutable workflow' >&2
  exit 1
fi
printf '%s\n' "$output"
```
Output:
```text
mutable GitHub Action references found:
.github/workflows/__supply-chain-regression.yml:7:      - uses: actions/checkout@v7
Pin each non-local action to a 40-character commit SHA.
```
Result: checker failed as expected; fixture removed by trap.

## 2026-08-21 Follow-up: round 2/5 checker hardening

### Summary
- Removed `.github/workflows` depth limit so nested workflow files are validated.
- Tightened Dockerfile installer rejection from direct `| sh/bash` wrappers to any `mise.run ... |` pipeline.
- Added `scripts/check-supply-chain-regression.sh` to exercise nested workflow discovery and Dockerfile `| env bash` / `| sudo sh` bypasses with auto-cleaned fixtures.

### RED: new regression harness against pre-fix checker
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && chmod +x scripts/check-supply-chain-regression.sh && bash scripts/check-supply-chain-regression.sh
```
Output:
```text
unexpected checker pass for nested workflow discovery
```
Result: exit 1; reproduced missed nested-workflow coverage before the checker fix.

### GREEN: regression harness after checker fix
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && bash scripts/check-supply-chain-regression.sh
```
Output:
```text
PASS nested workflow discovery
PASS Dockerfile mise.run | env bash
PASS Dockerfile mise.run | sudo sh
```
Result: exit 0.

### PASS: checker, syntax, config parse, regression, diff check
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && \
mise run supply-chain:check && \
bash -n scripts/check-supply-chain.sh scripts/check-supply-chain-regression.sh && \
python3 - <<'PY'
import tomllib, yaml
with open('mise.toml', 'rb') as f:
    tomllib.load(f)
print('TOML OK mise.toml')
for path in ['.github/workflows/ci.yml', '.github/workflows/release.yml', '.github/actions/go-cache/action.yml']:
    with open(path, 'r', encoding='utf-8') as f:
        yaml.safe_load(f)
    print(f'YAML OK {path}')
print('Shell OK scripts/check-supply-chain.sh')
print('Shell OK scripts/check-supply-chain-regression.sh')
PY
bash scripts/check-supply-chain-regression.sh && \
git --no-pager diff --check
```
Output:
```text
[supply-chain:check] $ scripts/check-supply-chain.sh
TOML OK mise.toml
YAML OK .github/workflows/ci.yml
YAML OK .github/workflows/release.yml
YAML OK .github/actions/go-cache/action.yml
Shell OK scripts/check-supply-chain.sh
Shell OK scripts/check-supply-chain-regression.sh
PASS nested workflow discovery
PASS Dockerfile mise.run | env bash
PASS Dockerfile mise.run | sudo sh
```
Result: exit 0; `git diff --check` produced no output.

### PASS: fixture cleanup
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && \
if [ -e .scratch/check-supply-chain-regression ]; then echo 'CLEANUP FAILED'; exit 1; fi && \
echo 'CLEANUP OK .scratch/check-supply-chain-regression removed'
```
Output:
```text
CLEANUP OK .scratch/check-supply-chain-regression removed
```
Result: exit 0.

#### RED: synthetic `version: latest`
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && fixture=.github/workflows/__compose-version-regression.yml && trap 'rm -f "$fixture"' EXIT && cat > "$fixture" <<'EOF'
name: regression
on: workflow_dispatch
jobs:
  mutable:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: docker/setup-compose-action@4eb059ff7f16592f9c84d5ca339c53cb7c5064e2 # v2
        with:
          version: latest
EOF
if output="$(bash scripts/check-supply-chain.sh 2>&1)"; then
  echo 'unexpected checker pass for synthetic latest compose version' >&2
  exit 1
fi
printf '%s\n' "$output"
```
Output:
```text
mutable Docker Compose version found:
.github/workflows/__compose-version-regression.yml:10:          version: latest
Pin docker/setup-compose-action version to an immutable release such as v5.5.0.
```
Result: checker failed as expected; fixture removed by trap.

#### RED: synthetic `mise.run | bash`
Command:
```bash
cd /home/canhdinh/workspace/simplebank/.worktrees/owasp-hardening && fixture=.github/workflows/__mise-pipe-regression.yml && trap 'rm -f "$fixture"' EXIT && cat > "$fixture" <<'EOF'
name: regression
on: workflow_dispatch
jobs:
  mutable:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - run: curl -fsSL https://mise.run | bash
EOF
if output="$(bash scripts/check-supply-chain.sh 2>&1)"; then
  echo 'unexpected checker pass for synthetic mise pipe installer' >&2
  exit 1
fi
printf '%s\n' "$output"
```
Output:
```text
mutable mise installer pipe found:
.github/workflows/__mise-pipe-regression.yml:8:      - run: curl -fsSL https://mise.run | bash
Download a versioned mise artifact and verify its checksum before use.
```
Result: checker failed as expected; fixture removed by trap.
