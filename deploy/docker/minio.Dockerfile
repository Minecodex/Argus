# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.25.8-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG MINIO_VERSION=RELEASE.2025-10-15T17-29-55Z

RUN apk add --no-cache ca-certificates git
WORKDIR /src
RUN --mount=type=cache,target=/root/.cache/git \
    git clone --depth 1 --branch "$MINIO_VERSION" https://github.com/minio/minio.git .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags kqueue -ldflags "-s -w" -o /out/minio .

FROM --platform=$BUILDPLATFORM golang:1.25.8-alpine AS mc-build
ARG TARGETOS
ARG TARGETARCH
ARG MINIO_MC_VERSION=RELEASE.2025-08-13T08-35-41Z
ARG MINIO_MC_COMMIT=7394ce0dd2a80935aded936b09fa12cbb3cb8096
RUN apk add --no-cache ca-certificates git
WORKDIR /src/mc
RUN git clone --depth 1 --branch "$MINIO_MC_VERSION" https://github.com/minio/mc.git . && \
    test "$(git rev-parse HEAD)" = "$MINIO_MC_COMMIT"
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    ldflags="$(MC_RELEASE=RELEASE go run buildscripts/gen-ldflags.go "${MINIO_MC_VERSION#RELEASE.}")" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "$ldflags" -o /out/mc .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S minio && adduser -S -G minio minio
COPY --from=build /out/minio /usr/local/bin/minio
COPY --from=mc-build /out/mc /usr/local/bin/mc
USER minio:minio
VOLUME ["/data"]
EXPOSE 9000 9001
ENTRYPOINT ["/usr/local/bin/minio"]
CMD ["server", "/data", "--console-address", ":9001"]
