ARG PLAN9_BASE=argus-plan9-host:b11c-base-20261115
FROM ${PLAN9_BASE}
COPY service.py mail.py /usr/local/lib/plan9-b13b/
COPY plan9-b13b.service /etc/systemd/system/plan9-b13b.service
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    systemctl enable plan9-b13b.service && mkdir -p /etc/systemd/journald.conf.d /etc/plan9 && chmod 700 /etc/plan9 && \
    printf '[Journal]\nSystemMaxUse=32M\nRuntimeMaxUse=16M\n' > /etc/systemd/journald.conf.d/plan9.conf
