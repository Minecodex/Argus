ARG PLAN9_BASE=argus-plan9-host:b01-20261026
FROM ${PLAN9_BASE}
COPY service.py /usr/local/lib/plan9-b11b.py
COPY plan9-b11b.service /etc/systemd/system/plan9-b11b.service
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    systemctl enable plan9-b11b.service && \
    mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nSystemMaxUse=32M\nRuntimeMaxUse=16M\n' > /etc/systemd/journald.conf.d/plan9.conf
