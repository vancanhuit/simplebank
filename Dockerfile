# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM debian:trixie-slim AS builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update  \
    && apt-get -y --no-install-recommends install  \
        curl git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV MISE_DATA_DIR="/mise"
ENV MISE_CONFIG_DIR="/mise"
ENV MISE_CACHE_DIR="/mise/cache"
ENV MISE_INSTALL_PATH="/usr/local/bin/mise"
ENV PATH="/mise/shims:$PATH"
ARG TARGETARCH
ARG MISE_VERSION=2026.8.10
RUN case "$TARGETARCH" in \
      amd64) mise_arch=x64; mise_sha=1f5e8795d24073904ef20ba70c1250ad6389d8c5672226d152e0ed24909ba72f ;; \
      arm64) mise_arch=arm64; mise_sha=57a14ecddf45aab8463a03bfbd424ebb08ba2d5808e19d45bd06d40c27019c4d ;; \
      *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac \
    && curl -fsSLo "$MISE_INSTALL_PATH" \
      "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-${mise_arch}" \
    && echo "${mise_sha}  ${MISE_INSTALL_PATH}" | sha256sum -c - \
    && chmod 0755 "$MISE_INSTALL_PATH"

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
ARG VERSION
ARG COMMIT
ARG BUILD_DATE
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.bun/install/cache \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    VERSION=$VERSION COMMIT=$COMMIT BUILD_DATE=$BUILD_DATE \
    mise run app:build

FROM gcr.io/distroless/base-debian13:nonroot
USER nonroot:nonroot
COPY --from=builder /app/dist/simplebank /simplebank
ENTRYPOINT ["/simplebank"]
