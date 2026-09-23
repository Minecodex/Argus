# syntax=docker/dockerfile:1.7
FROM golang:1.25.8-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/argus-replay-model ./cmd/argus-replay-model
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags=m4e2e -trimpath -ldflags "-s -w" -o /out/argus-replay-model ./cmd/argus-replay-model

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/argus-replay-model /usr/local/bin/argus-replay-model
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/argus-replay-model"]
