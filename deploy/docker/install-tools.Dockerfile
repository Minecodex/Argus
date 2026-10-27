# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/argus-install-kubectl ./cmd/argus-install-kubectl
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags '-s -w -X k8s.io/client-go/pkg/version.gitVersion=v1.35.3' \
    -o /out/kubectl ./cmd/argus-install-kubectl
FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
LABEL io.argus.image.purpose="installation"
COPY --from=build /out/kubectl /usr/local/bin/kubectl
ENV HOME=/tmp
WORKDIR /tmp
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kubectl"]
