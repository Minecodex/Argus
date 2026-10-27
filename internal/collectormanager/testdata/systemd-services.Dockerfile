ARG BASE_IMAGE=ubuntu:24.04
FROM ${BASE_IMAGE}
ENV container=docker
RUN apt-get update && apt-get install --no-install-recommends -y systemd systemd-sysv dbus && rm -rf /var/lib/apt/lists/*
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
