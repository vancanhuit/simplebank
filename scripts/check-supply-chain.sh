#!/usr/bin/env bash
set -euo pipefail

readonly mise_version="2026.8.9"
readonly mise_sha256="997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430"
readonly docker_version="29.7.2"
readonly buildx_version="v0.36.1"
readonly dockerfile_syntax="# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32"

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
  printf 'mutable Docker Compose version found:\n%s\nPin docker/setup-compose-action to an immutable release.\n' "$bad_compose_versions" >&2
  exit 1
fi

actual_syntax="$(head -n 1 Dockerfile)"
if [[ "$actual_syntax" != "$dockerfile_syntax" ]]; then
  printf 'Dockerfile syntax frontend must be digest pinned:\n%s\n' "$actual_syntax" >&2
  exit 1
fi

python3 - \
  "$mise_version" \
  "$mise_sha256" \
  "$docker_version" \
  "$buildx_version" \
  "${yaml_files[@]}" <<'PY'
from __future__ import annotations

import re
import sys
from pathlib import Path

mise_version, mise_sha256, docker_version, buildx_version, *file_names = sys.argv[1:]
paths = [Path(name) for name in file_names]


def indentation(line: str) -> int:
    return len(line) - len(line.lstrip())


def step_block(lines: list[str], start: int) -> list[str]:
    base_indent = indentation(lines[start])
    end = start + 1
    while end < len(lines):
        line = lines[end]
        if line.strip() and indentation(line) <= base_indent:
            break
        end += 1
    return lines[start:end]


def input_value(block: list[str], key: str) -> str | None:
    pattern = re.compile(rf"^\s+{re.escape(key)}:\s*([^\s#]+)")
    for line in block[1:]:
        match = pattern.match(line)
        if match:
            return match.group(1).strip("\"'")
    return None


def fail_group(label: str, failures: list[str]) -> None:
    if failures:
        print(f"{label}:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        raise SystemExit(1)


mise_failures: list[str] = []
docker_failures: list[str] = []
buildx_failures: list[str] = []

uses_pattern = re.compile(r"^\s*-\s+uses:\s+([^@\s]+)@([^\s#]+)")
for path in paths:
    lines = path.read_text().splitlines()
    for index, line in enumerate(lines):
        match = uses_pattern.match(line)
        if not match:
            continue
        action = match.group(1)
        block = step_block(lines, index)
        location = f"{path}:{index + 1}"
        if action == "jdx/mise-action":
            if (
                input_value(block, "version") != mise_version
                or input_value(block, "sha256") != mise_sha256
            ):
                mise_failures.append(location)
        elif action == "docker/setup-docker-action":
            if input_value(block, "version") != docker_version:
                docker_failures.append(location)
        elif action == "docker/setup-buildx-action":
            if input_value(block, "version") != buildx_version:
                buildx_failures.append(location)

fail_group("jdx/mise-action must pin version and sha256", mise_failures)
fail_group(
    f"docker/setup-docker-action must pin version {docker_version}",
    docker_failures,
)
fail_group(
    f"docker/setup-buildx-action must pin version {buildx_version}",
    buildx_failures,
)


def has_mise_pipeline(content: str) -> bool:
    normalized = re.sub(r"\\\s*\n", " ", content)
    return re.search(r"(?:https?://)?mise\.run\b.*\|", normalized, re.DOTALL) is not None


pipeline_failures: list[str] = []
docker_lines = Path("Dockerfile").read_text().splitlines()
index = 0
while index < len(docker_lines):
    line = docker_lines[index]
    if not re.match(r"^\s*RUN(?:\s|$)", line, re.IGNORECASE):
        index += 1
        continue
    start = index
    instruction = [line]
    while instruction[-1].rstrip().endswith("\\") and index + 1 < len(docker_lines):
        index += 1
        instruction.append(docker_lines[index])
    if has_mise_pipeline("\n".join(instruction)):
        pipeline_failures.append(f"Dockerfile:{start + 1}")
    index += 1

run_pattern = re.compile(r"^(\s*)(?:-\s*)?run:\s*(.*)$")
for path in paths:
    lines = path.read_text().splitlines()
    index = 0
    while index < len(lines):
        match = run_pattern.match(lines[index])
        if not match:
            index += 1
            continue
        start = index
        base_indent = indentation(lines[index])
        value = match.group(2).strip()
        content: list[str] = []
        if value in {"|", ">", "|-", ">-", "|+", ">+"}:
            index += 1
            while index < len(lines):
                line = lines[index]
                if line.strip() and indentation(line) <= base_indent:
                    break
                content.append(line)
                index += 1
        else:
            content.append(value)
            index += 1
        if has_mise_pipeline("\n".join(content)):
            pipeline_failures.append(f"{path}:{start + 1}")

fail_group("mutable mise installer pipeline found", pipeline_failures)


def parse_needs(value: str) -> list[str]:
    value = value.strip()
    if not value:
        return []
    if value.startswith("[") and value.endswith("]"):
        value = value[1:-1]
        return [item.strip().strip("\"'") for item in value.split(",") if item.strip()]
    return [value.strip("\"'")]


def workflow_jobs(path: Path) -> tuple[dict[str, list[str]], dict[str, str]]:
    lines = path.read_text().splitlines()
    jobs_start = next(
        (index for index, line in enumerate(lines) if re.match(r"^jobs:\s*$", line)),
        None,
    )
    if jobs_start is None:
        return {}, {}

    starts: list[tuple[str, int]] = []
    for index in range(jobs_start + 1, len(lines)):
        line = lines[index]
        if line.strip() and indentation(line) == 0:
            break
        match = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
        if match:
            starts.append((match.group(1), index))

    needs: dict[str, list[str]] = {}
    bodies: dict[str, str] = {}
    for position, (job, start) in enumerate(starts):
        end = starts[position + 1][1] if position + 1 < len(starts) else len(lines)
        block = lines[start:end]
        bodies[job] = "\n".join(block)
        job_needs: list[str] = []
        for offset, line in enumerate(block[1:], start=1):
            match = re.match(r"^    needs:\s*(.*)$", line)
            if not match:
                continue
            job_needs = parse_needs(match.group(1))
            if not job_needs:
                next_index = offset + 1
                while next_index < len(block) and indentation(block[next_index]) > 4:
                    item = re.match(r"^\s*-\s*([A-Za-z0-9_-]+)\s*$", block[next_index])
                    if item:
                        job_needs.append(item.group(1))
                    next_index += 1
            break
        needs[job] = job_needs
    return needs, bodies


def reaches_supply_chain(job: str, needs: dict[str, list[str]], seen: set[str]) -> bool:
    if job == "supply-chain":
        return True
    if job in seen:
        return False
    seen.add(job)
    return any(
        dependency in needs
        and reaches_supply_chain(dependency, needs, seen.copy())
        for dependency in needs.get(job, [])
    )


for workflow_name in (".github/workflows/ci.yml", ".github/workflows/release.yml"):
    path = Path(workflow_name)
    if not path.exists():
        continue
    needs, bodies = workflow_jobs(path)
    if "supply-chain" not in needs:
        print(f"workflow must define a supply-chain job:\n{path}", file=sys.stderr)
        raise SystemExit(1)
    supply_body = bodies["supply-chain"]
    if "scripts/check-supply-chain.sh" not in supply_body:
        print(f"supply-chain job must run scripts/check-supply-chain.sh:\n{path}", file=sys.stderr)
        raise SystemExit(1)
    if "scripts/check-supply-chain-regression.sh" not in supply_body:
        print(
            "supply-chain job must run scripts/check-supply-chain-regression.sh:"
            f"\n{path}",
            file=sys.stderr,
        )
        raise SystemExit(1)

    ungated = sorted(
        job
        for job in needs
        if job != "supply-chain" and not reaches_supply_chain(job, needs, set())
    )
    if ungated:
        print(
            "workflow jobs are not gated by supply-chain:"
            f"\n{path}: {', '.join(ungated)}",
            file=sys.stderr,
        )
        raise SystemExit(1)

    if path.name == "release.yml":
        for job in ("binaries", "docker"):
            if job not in needs or "supply-chain" not in needs[job]:
                print(
                    "release binaries and docker jobs must directly need supply-chain:"
                    f"\n{path}: {job}",
                    file=sys.stderr,
                )
                raise SystemExit(1)
        if not {"binaries", "docker"}.issubset(set(needs.get("release", []))):
            print(
                "release job must retain binaries and docker dependencies:"
                f"\n{path}: release",
                file=sys.stderr,
            )
            raise SystemExit(1)
PY

if ! grep -Eq '/usr/local/bin/mise.*\|[[:space:]]*sha256sum -c -' Dockerfile; then
  die 'Dockerfile must verify the downloaded mise binary checksum for /usr/local/bin/mise'
fi

from_count="$(grep -c '^FROM ' Dockerfile)"
pinned_from_count="$(grep -cE '^FROM .*@sha256:[0-9a-f]{64}' Dockerfile)"
if [[ "$from_count" != "$pinned_from_count" ]]; then
  die 'every Dockerfile base image must be digest pinned'
fi
