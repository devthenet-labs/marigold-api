# Copyright 2026 DevTheNet Labs.
# SPDX-License-Identifier: MIT
# The agent's toolchain image: patchy's agent base plus the pinned Go
# toolchain, offline. Application source is never baked into it.
FROM golang:1.26.6@sha256:0d1d3a794be25f809dd2cb3160d8c73276c4056a9f8242a138e908ddeee7b6b6 AS toolchain
FROM ghcr.io/devthenet-labs/patchy/agent-base:v0.12.13@sha256:b74ed86f003954a587da9aa50fa437362b7d2837f9fa0a02552c5b8be30d1493
USER 0:0
COPY --from=toolchain /usr/local/go /usr/local/go
RUN mkdir -p /usr/local/bin \
    && ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt \
    && mkdir -p /opt/go/pkg/mod
ENV GOTOOLCHAIN=local GOMODCACHE=/opt/go/pkg/mod GOPROXY=off CGO_ENABLED=0
WORKDIR /tmp/deps
COPY go.mod ./
RUN go mod download
LABEL org.opencontainers.image.source="https://github.com/devthenet-labs/marigold-api" \
      org.opencontainers.image.title="marigold-api agent toolchain"
WORKDIR /workspace
USER 65532:65532
