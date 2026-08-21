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
EOF

  cat > "$fixture_dir/Dockerfile" <<'EOF'
FROM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder
RUN curl -fsSLo /usr/local/bin/mise "https://github.com/jdx/mise/releases/download/v2026.8.9/mise-v2026.8.9-linux-x64" \
    && echo "997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430  /usr/local/bin/mise" | sha256sum -c -
FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
EOF
}

run_expect_fail() {
  local case_name="$1"
  local fixture_dir="$2"
  local expected_message="$3"
  local expected_line="$4"
  local output

  if output="$(cd "$fixture_dir" && bash "$checker" 2>&1)"; then
    printf 'unexpected checker pass for %s\n' "$case_name" >&2
    exit 1
  fi

  assert_contains "$output" "$expected_message" "$case_name"
  assert_contains "$output" "$expected_line" "$case_name"
  printf 'PASS %s\n' "$case_name"
}

mkdir -p "$scratch_dir"

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

env_bash_fixture="$scratch_dir/dockerfile-env-bash"
write_baseline_fixture "$env_bash_fixture"
cat > "$env_bash_fixture/Dockerfile" <<'EOF'
FROM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder
RUN curl -fsSL https://mise.run | env bash
FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
EOF
run_expect_fail \
  "Dockerfile mise.run | env bash" \
  "$env_bash_fixture" \
  "mutable mise installer pipe found:" \
  "Dockerfile:2:RUN curl -fsSL https://mise.run | env bash"

sudo_sh_fixture="$scratch_dir/dockerfile-sudo-sh"
write_baseline_fixture "$sudo_sh_fixture"
cat > "$sudo_sh_fixture/Dockerfile" <<'EOF'
FROM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder
RUN curl -fsSL https://mise.run | sudo sh
FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
EOF
run_expect_fail \
  "Dockerfile mise.run | sudo sh" \
  "$sudo_sh_fixture" \
  "mutable mise installer pipe found:" \
  "Dockerfile:2:RUN curl -fsSL https://mise.run | sudo sh"
