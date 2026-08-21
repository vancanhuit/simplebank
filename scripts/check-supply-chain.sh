#!/usr/bin/env bash
set -euo pipefail

workflow_files=(
  .github/workflows/ci.yml
  .github/workflows/release.yml
  .github/actions/go-cache/action.yml
)

bad_actions="$(
  grep -HnE '^[[:space:]]*(- )?uses:' "${workflow_files[@]}" \
    | grep -Ev 'uses:[[:space:]]+\./|uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}([[:space:]]+#.*)?$' \
    || true
)"
if [[ -n "$bad_actions" ]]; then
  printf 'mutable GitHub Action references:\n%s\n' "$bad_actions" >&2
  exit 1
fi

if grep -Fq 'curl https://mise.run | sh' Dockerfile; then
  echo 'Dockerfile executes mutable mise installer' >&2
  exit 1
fi

grep -Fq 'sha256sum -c -' Dockerfile

from_count="$(grep -c '^FROM ' Dockerfile)"
pinned_from_count="$(grep -cE '^FROM .*@sha256:[0-9a-f]{64}' Dockerfile)"
if [[ "$from_count" != "$pinned_from_count" ]]; then
  echo 'every Dockerfile base image must be digest pinned' >&2
  exit 1
fi
