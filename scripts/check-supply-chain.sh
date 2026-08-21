#!/usr/bin/env bash
set -euo pipefail

die() {
  printf '%s\n' "$1" >&2
  exit 1
}

workflow_files=()
while IFS= read -r -d '' file; do
  workflow_files+=("$file")
done < <(
  if [[ -d .github/workflows ]]; then
    find .github/workflows -type f \( -name '*.yml' -o -name '*.yaml' \) -print0 | sort -z
  fi
)

action_files=()
while IFS= read -r -d '' file; do
  action_files+=("$file")
done < <(
  if [[ -d .github/actions ]]; then
    find .github/actions -type f \( -name 'action.yml' -o -name 'action.yaml' \) -print0 | sort -z
  fi
)

yaml_files=("${workflow_files[@]}" "${action_files[@]}")
if [[ ${#yaml_files[@]} -eq 0 ]]; then
  die 'no GitHub workflow or action metadata files found to validate'
fi

bad_actions="$(
  grep -HnE '^[[:space:]]*(- )?uses:' "${yaml_files[@]}" \
    | grep -Ev 'uses:[[:space:]]+\./|uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}([[:space:]]+#.*)?$' \
    || true
)"
if [[ -n "$bad_actions" ]]; then
  printf 'mutable GitHub Action references found:\n%s\nPin each non-local action to a 40-character commit SHA.\n' "$bad_actions" >&2
  exit 1
fi

bad_compose_versions="$(
  grep -HnE '^[[:space:]]*version:[[:space:]]+latest([[:space:]]+#.*)?$' "${yaml_files[@]}" || true
)"
if [[ -n "$bad_compose_versions" ]]; then
  printf 'mutable Docker Compose version found:\n%s\nPin docker/setup-compose-action version to an immutable release such as v5.5.0.\n' "$bad_compose_versions" >&2
  exit 1
fi

bad_mise_pipes="$(
  grep -HnE 'mise\.run.*\|' Dockerfile || true
)"
if [[ -n "$bad_mise_pipes" ]]; then
  printf 'mutable mise installer pipe found:\n%s\nDownload a versioned mise artifact and verify its checksum before use.\n' "$bad_mise_pipes" >&2
  exit 1
fi

if ! grep -Eq '/usr/local/bin/mise.*\|[[:space:]]*sha256sum -c -' Dockerfile; then
  die 'Dockerfile must verify the downloaded mise binary checksum for /usr/local/bin/mise'
fi

from_count="$(grep -c '^FROM ' Dockerfile)"
pinned_from_count="$(grep -cE '^FROM .*@sha256:[0-9a-f]{64}' Dockerfile)"
if [[ "$from_count" != "$pinned_from_count" ]]; then
  die 'every Dockerfile base image must be digest pinned'
fi
