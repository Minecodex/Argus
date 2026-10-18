# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
ARG GO_BUILD_TAGS=""
COPY api/openapi ./api/openapi
COPY api/schemas ./api/schemas
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    mkdir -p /out && \
    for name in argus-server argus-worker argus-connector-gateway argus-telemetry argus-telemetry-dlq-replay argus-connector argus-remote-runtime argus-pki-controller argusctl argus-migrate argus-telemetry-catalog-sync argus-sandbox-smoke; do \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
		-tags "$GO_BUILD_TAGS" \
        -trimpath \
        -ldflags "-s -w -X github.com/kakj-go/Argus/internal/buildinfo.Version=$VERSION -X github.com/kakj-go/Argus/internal/buildinfo.Commit=$COMMIT -X github.com/kakj-go/Argus/internal/buildinfo.Date=$BUILD_DATE" \
        -o /out/$name ./cmd/$name; \
    done

RUN mkdir -p /helpers && cp /out/argus-remote-runtime /helpers/linux-${TARGETARCH}

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ -n "$GO_BUILD_TAGS" ]; then \
      CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
		-tags "$GO_BUILD_TAGS" -trimpath -ldflags "-s -w" \
		-o /out/argus-telemetry-e2e ./cmd/argus-telemetry-e2e; \
    fi

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /usr/local/bin/
COPY --from=build /helpers/ /usr/local/lib/argus/remote-runtime/
COPY --from=build /src/migrations/ /usr/local/share/argus/migrations/
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/argus-server"]
