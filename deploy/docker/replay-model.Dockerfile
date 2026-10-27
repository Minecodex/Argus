# syntax=mirror.gcr.io/docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
FROM mirror.gcr.io/library/golang:1.25.8-alpine@sha256:8e02eb337d9e0ea459e041f1ee5eece41cbb61f1d83e7d883a3e2fb4862063fa AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/argus-replay-model ./cmd/argus-replay-model
COPY internal/integration/modelprovider ./internal/integration/modelprovider
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags=m4e2e -trimpath -ldflags "-s -w" -o /out/argus-replay-model ./cmd/argus-replay-model

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/argus-replay-model /usr/local/bin/argus-replay-model
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/argus-replay-model"]
