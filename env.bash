#!/usr/bin/env bash

VERSION="${VERSION:-$(git describe --tags --always --dirty=-dev 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

export VERSION
export COMMIT
export BUILD_DATE
