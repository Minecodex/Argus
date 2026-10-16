# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS build

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

FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS mc-build
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

FROM mirror.gcr.io/library/alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8
RUN apk add --no-cache ca-certificates && addgroup -S minio && adduser -S -G minio minio
COPY --from=build /out/minio /usr/local/bin/minio
COPY --from=mc-build /out/mc /usr/local/bin/mc
USER minio:minio
VOLUME ["/data"]
EXPOSE 9000 9001
ENTRYPOINT ["/usr/local/bin/minio"]
CMD ["server", "/data", "--console-address", ":9001"]
