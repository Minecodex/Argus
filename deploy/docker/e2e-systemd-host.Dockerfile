# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.25.8-alpine AS generator

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/argus-telemetry-e2e ./cmd/argus-telemetry-e2e
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags m4e2e -trimpath -ldflags "-s -w" \
      -o /out/argus-telemetry-e2e ./cmd/argus-telemetry-e2e

FROM ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3

ENV container=docker
# The minimal Ubuntu layer has no CA bundle yet. Bootstrap from the trusted
# builder so package acquisition can use verified HTTPS before installing it.
COPY --from=generator /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN sed -i 's|http://archive.ubuntu.com/ubuntu|https://archive.ubuntu.com/ubuntu|g;s|http://security.ubuntu.com/ubuntu|https://security.ubuntu.com/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources && \
    apt-get -o Acquire::Retries=3 update && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=3 install -y --no-install-recommends \
      ca-certificates curl iproute2 iptables openssh-client openssh-server openssl socat sudo systemd systemd-sysv && \
    useradd --create-home --shell /bin/bash argus && \
    echo 'argus:M3-e2e-ssh-password' | chpasswd && \
	echo 'root:M3-e2e-ssh-password' | chpasswd && \
    mkdir -p /run/sshd && \
	mkdir -p /var/log/journal && \
	printf 'PasswordAuthentication yes\nPermitRootLogin yes\n' >/etc/ssh/sshd_config.d/argus-e2e.conf && \
    systemctl enable ssh.service && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=generator /out/argus-telemetry-e2e /usr/local/bin/argus-telemetry-e2e

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
