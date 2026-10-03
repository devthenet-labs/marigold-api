# Copyright 2026 DevTheNet Labs.
# SPDX-License-Identifier: MIT
# Runtime image: a static binary on distroless (non-root uid 65532, no shell),
# listening on 8080. It writes nothing, so it runs with a read-only root.
FROM golang:1.26.6@sha256:0d1d3a794be25f809dd2cb3160d8c73276c4056a9f8242a138e908ddeee7b6b6 AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
ARG BUILD_SHA=dev
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.commitSHA=${BUILD_SHA}" -o /out/marigold-api .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/marigold-api /marigold-api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/marigold-api"]
