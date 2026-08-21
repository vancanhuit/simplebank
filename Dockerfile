# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32
FROM --platform=$BUILDPLATFORM debian:trixie-slim@sha256:3a39a0592364683e6bab97937b72cad5a8fa6dcbbee90edb3bb48c7f8e94f258 AS builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update  \
    && apt-get -y --no-install-recommends install  \
        curl git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV MISE_DATA_DIR="/mise"
ENV MISE_CONFIG_DIR="/mise"
ENV MISE_CACHE_DIR="/mise/cache"
ENV PATH="/mise/shims:$PATH"

ARG BUILDARCH
ARG MISE_VERSION=v2026.8.9
ARG MISE_SHA256_X64=997b6f3e0d760d292eb99f2824c70dddb432514b3a0d487f975c0f22b3cae430
ARG MISE_SHA256_ARM64=8ad1ecc90aa40b234e96d77b62af94b6f40a59b4b1527dc1b075c007e54407c7

RUN case "$BUILDARCH" in \
      amd64) mise_arch=x64; mise_sha="$MISE_SHA256_X64" ;; \
      arm64) mise_arch=arm64; mise_sha="$MISE_SHA256_ARM64" ;; \
      *) echo "unsupported BUILDARCH: $BUILDARCH" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo /usr/local/bin/mise \
      "https://github.com/jdx/mise/releases/download/${MISE_VERSION}/mise-${MISE_VERSION}-linux-${mise_arch}" \
    && echo "${mise_sha}  /usr/local/bin/mise" | sha256sum -c - \
    && chmod 0755 /usr/local/bin/mise

WORKDIR /app

COPY mise.toml .
COPY mise.lock .
RUN --mount=type=cache,target=/mise/cache \
    mise trust && mise install go bun

COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY sqlc.yaml sqlc.yaml

COPY cmd cmd
COPY internal internal

COPY frontend/ frontend/

ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT
ARG BUILD_DATE
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,id=bun-install-${TARGETARCH},target=/root/.bun/install/cache \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    VERSION=$VERSION COMMIT=$COMMIT BUILD_DATE=$BUILD_DATE \
    mise run app:build

FROM gcr.io/distroless/base-debian13:nonroot@sha256:97b9d04bed1c754b756c3c4b6a04915c22fb0b5d96a59944eb3bf78c26e6e157
USER nonroot:nonroot
COPY --from=builder /app/dist/simplebank /simplebank
ENTRYPOINT ["/simplebank"]
