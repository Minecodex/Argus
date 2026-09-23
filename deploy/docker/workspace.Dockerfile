# syntax=docker/dockerfile:1.7
FROM node:24.20.0-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS node
FROM --platform=$BUILDPLATFORM golang:1.25.8-alpine AS supervisor
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api ./api
COPY internal ./internal
COPY cmd/argus-workspace-supervisor ./cmd/argus-workspace-supervisor
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags '-s -w' -o /out/supervisor ./cmd/argus-workspace-supervisor
FROM python:3.12.12-slim-bookworm@sha256:593bd06efe90efa80dc4eee3948be7c0fde4134606dd40d8dd8dbcade98e669c
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY deploy/workspace/requirements.lock /opt/argus/requirements.lock
RUN pip install --no-cache-dir --only-binary=:all: --require-hashes -r /opt/argus/requirements.lock \
    && python -c 'import numpy,pandas,scipy,matplotlib,openpyxl,pyarrow' \
    && node --version && bash --version \
    && mkdir -p /workspace /home/argus && chmod 1777 /workspace /home/argus
COPY --from=supervisor /out/supervisor /usr/local/bin/argus-workspace-supervisor
RUN mkdir -p /var/run/argus/manager/tls && chown 10001:10001 /var/run/argus/manager && chmod 0700 /var/run/argus/manager \
    && python -c "import os; os.setxattr('/usr/local/bin/argus-workspace-supervisor', 'security.capability', bytes.fromhex('01000002e0000000000000000000000000000000'))"
ENV HOME=/home/argus MPLCONFIGDIR=/home/argus/.matplotlib PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1
WORKDIR /workspace
USER 100000:100000
CMD ["/bin/bash"]
