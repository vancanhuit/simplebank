# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM debian:trixie-slim AS builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update  \
    && apt-get -y --no-install-recommends install  \
        # install any other dependencies you might need
        sudo curl git ca-certificates build-essential \
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
    mise trust && mise install go

COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY cmd cmd
COPY internal internal

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    GOOS=$TARGETOS GOARCH=$TARGETARCH mise run app:build

FROM gcr.io/distroless/base-debian13
COPY --from=builder /app/dist/simplebank /simplebank
ENTRYPOINT ["/simplebank"]
