ARG PLAN9_BASE=argus-plan9-host:b11c-base-20261115
FROM ${PLAN9_BASE}
COPY service.py redis_client.py /usr/local/lib/plan9-b12/
COPY plan9-b12.service /etc/systemd/system/plan9-b12.service
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    systemctl enable plan9-b12.service && mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nSystemMaxUse=32M\nRuntimeMaxUse=16M\n' > /etc/systemd/journald.conf.d/plan9.conf
