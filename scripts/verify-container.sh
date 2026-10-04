#!/usr/bin/env bash
# Verify loaded multi-platform images without publishing or starting services.
set -euo pipefail

image=${1:-simplebank:latest}
scratch=$(mktemp -d)
container=
cleanup() {
    if [[ -n "$container" ]]; then
        docker rm -f "$container" >/dev/null
    fi
    rm -rf "$scratch"
}
trap cleanup EXIT

for arch in amd64 arm64; do
    container=$(docker create --platform "linux/$arch" --network none "$image" version)
    user=$(docker inspect --format '{{.Config.User}}' "$container")
    [[ "$user" == nonroot:nonroot ]] || {
        printf 'Unexpected runtime user: %s\n' "$user" >&2
        exit 1
    }
    docker cp "$container:/simplebank" "$scratch/simplebank"
    description=$(file -b "$scratch/simplebank")
    case "$arch:$description" in
    amd64:ELF\ 64-bit\ LSB*executable*'x86-64'*) ;;
    arm64:ELF\ 64-bit\ LSB*executable*'ARM aarch64'*) ;;
    *)
        printf 'Wrong executable for %s: %s\n' "$arch" "$description" >&2
        exit 1
        ;;
    esac
    printf '%s: %s\n' "$arch" "$description"
    timeout 30s docker start -a "$container"
    [[ $(docker inspect --format '{{.State.ExitCode}}' "$container") == 0 ]]
    docker rm "$container" >/dev/null
    container=
done
