# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api ./api
COPY internal ./internal
COPY cmd/argus-remote-runtime ./cmd/argus-remote-runtime
COPY cmd/argus-kubectl ./cmd/argus-kubectl
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags '-s -w' -o /out/argus-remote-runtime ./cmd/argus-remote-runtime
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags '-s -w -X k8s.io/client-go/pkg/version.gitVersion=v1.35.3' -o /out/kubectl ./cmd/argus-kubectl
FROM python:3.12.12-slim-bookworm@sha256:593bd06efe90efa80dc4eee3948be7c0fde4134606dd40d8dd8dbcade98e669c
COPY --from=build /out/argus-remote-runtime /out/kubectl /usr/local/bin/
RUN echo 'argus:x:65532:65532:Argus remote runtime:/tmp:/bin/bash' >> /etc/passwd \
    && echo 'argus:x:65532:' >> /etc/group && bash --version && kubectl version --client
ENV HOME=/tmp
WORKDIR /tmp
USER 65532:65532
ENTRYPOINT ["/bin/bash"]
