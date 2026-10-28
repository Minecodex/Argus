ARG TRUST_IMAGE=localhost:5001/argus/argus-remote-runtime@sha256:8a2366f74b988ead98caf266c8fc5f0fd2d125df64c1f0cf4bd1372f33553872
FROM ${TRUST_IMAGE} AS trust
FROM debian:13-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad

ENV container=docker
COPY --from=trust /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN sed -i 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/debian.sources && \
    apt-get -o Acquire::Retries=3 -o Acquire::https::CaInfo=/etc/ssl/certs/ca-certificates.crt update && \
    DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=3 -o Acquire::https::CaInfo=/etc/ssl/certs/ca-certificates.crt install -y --no-install-recommends \
      ca-certificates curl dbus iproute2 iputils-ping openssh-client openssh-server \
      openssl procps python3 sudo systemd systemd-sysv && \
    passwd -l root && mkdir -p /run/sshd /var/lib/plan9 /var/log/journal && \
    printf 'PasswordAuthentication yes\nPermitRootLogin yes\n' > /etc/ssh/sshd_config.d/plan9.conf && \
    systemctl enable ssh.service && \
    rm -f /etc/ssh/ssh_host_* /var/lib/dbus/machine-id && \
    truncate -s 0 /etc/machine-id && ln -s /etc/machine-id /var/lib/dbus/machine-id && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
COPY bootstrap.sh /usr/local/bin/plan9-bootstrap
COPY workload.py /usr/local/bin/plan9-workload
COPY systemd/ /etc/systemd/system/
RUN chmod 755 /usr/local/bin/plan9-bootstrap /usr/local/bin/plan9-workload && \
    chmod 644 /etc/systemd/system/plan9-*.service && \
    systemctl enable plan9-web.service plan9-anchor.service plan9-traffic.service
STOPSIGNAL SIGRTMIN+3
ENTRYPOINT ["/usr/local/bin/plan9-bootstrap"]
