# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY tests/e2e/sshserver ./tests/e2e/sshserver
COPY tests/e2e/remoteclient ./tests/e2e/remoteclient
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/argus-e2e-ssh ./tests/e2e/sshserver && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/argus-e2e-remoteclient ./tests/e2e/remoteclient

FROM mirror.gcr.io/library/alpine:3.22.1@sha256:4bcff63911fcb4448bd4fdacec207030997caf25e9bea4045fa6c8c44de311d1
RUN apk add --no-cache iproute2
COPY --from=build /out/argus-e2e-ssh /usr/local/bin/argus-e2e-ssh
COPY --from=build /out/argus-e2e-remoteclient /usr/local/bin/argus-e2e-remoteclient
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/argus-e2e-ssh"]
