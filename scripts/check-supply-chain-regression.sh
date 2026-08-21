#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
checker="$root_dir/scripts/check-supply-chain.sh"
scratch_dir="$root_dir/.scratch/check-supply-chain-regression"

cleanup() {
  rm -rf "$scratch_dir"
}
trap cleanup EXIT

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"

  if [[ "$haystack" != *"$needle"* ]]; then
    printf 'expected %s to contain:\n%s\nfull output:\n%s\n' "$label" "$needle" "$haystack" >&2
    exit 1
  fi
}

write_baseline_fixture() {
  local fixture_dir="$1"

  mkdir -p "$fixture_dir/.github/workflows"

  cat > "$fixture_dir/.github/workflows/pinned.yml" <<'EOF'
name: pinned
on: workflow_dispatch
jobs:
  pinned:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7
      - uses: jdx/mise-action@3c2e0cf82a5b2e5249f0d3635a4d83d0ae861518 # v4
        with:
          version: 2026.8.9
          sha256: 997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430
      - uses: docker/setup-docker-action@77e84dbf09b47d1e29270283c22f16145aa85ca1 # v5
        with:
          version: 29.7.2
          daemon-config: |
            {"features":{"containerd-snapshotter":true}}
      - uses: docker/setup-buildx-action@37fe631027851001ddb9b187196cc803df7f5f0e # v4
        with:
          version: v0.36.1
EOF

  cat > "$fixture_dir/Dockerfile" <<'EOF'
# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
FROM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder
RUN curl -fsSLo /usr/local/bin/mise "https://github.com/jdx/mise/releases/download/v2026.8.9/mise-v2026.8.9-linux-x64" \
    && echo "997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430  /usr/local/bin/mise" | sha256sum -c -
FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
EOF
}

run_expect_pass() {
  local case_name="$1"
  local fixture_dir="$2"
  local output

  if ! output="$(cd "$fixture_dir" && bash "$checker" 2>&1)"; then
    printf 'unexpected checker failure for %s:\n%s\n' "$case_name" "$output" >&2
    exit 1
  fi
  printf 'PASS %s\n' "$case_name"
}

run_expect_fail() {
  local case_name="$1"
  local fixture_dir="$2"
  local expected_message="$3"
  local expected_detail="$4"
  local output

  if output="$(cd "$fixture_dir" && bash "$checker" 2>&1)"; then
    printf 'unexpected checker pass for %s\n' "$case_name" >&2
    exit 1
  fi

  assert_contains "$output" "$expected_message" "$case_name"
  assert_contains "$output" "$expected_detail" "$case_name"
  printf 'PASS %s\n' "$case_name"
}

mkdir -p "$scratch_dir"

baseline_fixture="$scratch_dir/baseline"
write_baseline_fixture "$baseline_fixture"
run_expect_pass "fully pinned baseline" "$baseline_fixture"

nested_fixture="$scratch_dir/nested-workflow"
write_baseline_fixture "$nested_fixture"
mkdir -p "$nested_fixture/.github/workflows/nested"
cat > "$nested_fixture/.github/workflows/nested/mutable.yml" <<'EOF'
name: nested
on: workflow_dispatch
jobs:
  mutable:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
EOF
run_expect_fail \
  "nested workflow discovery" \
  "$nested_fixture" \
  "mutable GitHub Action references found:" \
  ".github/workflows/nested/mutable.yml:7:      - uses: actions/checkout@v7"

mise_defaults_fixture="$scratch_dir/mise-defaults"
write_baseline_fixture "$mise_defaults_fixture"
python3 - "$mise_defaults_fixture/.github/workflows/pinned.yml" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text()
text = text.replace(
    """      - uses: jdx/mise-action@3c2e0cf82a5b2e5249f0d3635a4d83d0ae861518 # v4
        with:
          version: 2026.8.9
          sha256: 997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430
""",
    "      - uses: jdx/mise-action@3c2e0cf82a5b2e5249f0d3635a4d83d0ae861518 # v4\n",
)
path.write_text(text)
PY
run_expect_fail \
  "mise action executable defaults" \
  "$mise_defaults_fixture" \
  "jdx/mise-action must pin version and sha256:" \
  ".github/workflows/pinned.yml:8"

docker_defaults_fixture="$scratch_dir/docker-defaults"
write_baseline_fixture "$docker_defaults_fixture"
sed -i '/^          version: 29\.7\.2$/d' "$docker_defaults_fixture/.github/workflows/pinned.yml"
run_expect_fail \
  "Docker CE executable default" \
  "$docker_defaults_fixture" \
  "docker/setup-docker-action must pin version 29.7.2:" \
  ".github/workflows/pinned.yml:12"

buildx_defaults_fixture="$scratch_dir/buildx-defaults"
write_baseline_fixture "$buildx_defaults_fixture"
sed -i '/^          version: v0\.36\.1$/d' "$buildx_defaults_fixture/.github/workflows/pinned.yml"
run_expect_fail \
  "Buildx executable default" \
  "$buildx_defaults_fixture" \
  "docker/setup-buildx-action must pin version v0.36.1:" \
  ".github/workflows/pinned.yml:17"

dockerfile_syntax_fixture="$scratch_dir/dockerfile-syntax"
write_baseline_fixture "$dockerfile_syntax_fixture"
sed -i '1c# syntax=docker/dockerfile:1' "$dockerfile_syntax_fixture/Dockerfile"
run_expect_fail \
  "mutable Dockerfile frontend syntax" \
  "$dockerfile_syntax_fixture" \
  "Dockerfile syntax frontend must be digest pinned:" \
  "# syntax=docker/dockerfile:1"

env_bash_fixture="$scratch_dir/dockerfile-env-bash"
write_baseline_fixture "$env_bash_fixture"
cat > "$env_bash_fixture/Dockerfile" <<'EOF'
# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
FROM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder
RUN curl -fsSL https://mise.run \
    | env bash
FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
EOF
run_expect_fail \
  "Dockerfile mise.run pipeline" \
  "$env_bash_fixture" \
  "mutable mise installer pipeline found:" \
  "Dockerfile:3"

workflow_pipe_fixture="$scratch_dir/workflow-mise-pipe"
write_baseline_fixture "$workflow_pipe_fixture"
cat > "$workflow_pipe_fixture/.github/workflows/mutable-pipe.yml" <<'EOF'
name: mutable pipe
on: workflow_dispatch
jobs:
  mutable:
    runs-on: ubuntu-latest
    steps:
      - run: curl -fsSL https://mise.run | sh
EOF
run_expect_fail \
  "workflow mise.run pipeline" \
  "$workflow_pipe_fixture" \
  "mutable mise installer pipeline found:" \
  ".github/workflows/mutable-pipe.yml:7"

composite_pipe_fixture="$scratch_dir/composite-mise-pipe"
write_baseline_fixture "$composite_pipe_fixture"
mkdir -p "$composite_pipe_fixture/.github/actions/mutable"
cat > "$composite_pipe_fixture/.github/actions/mutable/action.yml" <<'EOF'
name: mutable
description: mutable installer
runs:
  using: composite
  steps:
    - shell: bash
      run: |
        curl -fsSL https://mise.run \
          | sudo sh
EOF
run_expect_fail \
  "composite mise.run pipeline" \
  "$composite_pipe_fixture" \
  "mutable mise installer pipeline found:" \
  ".github/actions/mutable/action.yml:7"

ungated_fixture="$scratch_dir/ungated-ci"
write_baseline_fixture "$ungated_fixture"
cat > "$ungated_fixture/.github/workflows/ci.yml" <<'EOF'
name: CI
on: pull_request
jobs:
  supply-chain:
    runs-on: ubuntu-latest
    steps:
      - run: scripts/check-supply-chain.sh
      - run: scripts/check-supply-chain-regression.sh
  unit:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
EOF
run_expect_fail \
  "ungated CI root job" \
  "$ungated_fixture" \
  "workflow jobs are not gated by supply-chain:" \
  ".github/workflows/ci.yml: unit"

missing_regression_fixture="$scratch_dir/missing-regression-step"
write_baseline_fixture "$missing_regression_fixture"
cat > "$missing_regression_fixture/.github/workflows/release.yml" <<'EOF'
name: Release
on:
  push:
    tags: ["v*"]
jobs:
  supply-chain:
    runs-on: ubuntu-latest
    steps:
      - run: scripts/check-supply-chain.sh
EOF
run_expect_fail \
  "missing committed regression execution" \
  "$missing_regression_fixture" \
  "supply-chain job must run scripts/check-supply-chain-regression.sh:" \
  ".github/workflows/release.yml"
