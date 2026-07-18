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
RUN curl https://mise.run | sh

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
    --mount=type=cache,target=/root/.bun/install/cache \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    VERSION=$VERSION COMMIT=$COMMIT BUILD_DATE=$BUILD_DATE \
    mise run app:build

FROM gcr.io/distroless/base-debian13:nonroot
USER nonroot:nonroot
COPY --from=builder /app/dist/simplebank /simplebank
ENTRYPOINT ["/simplebank"]
