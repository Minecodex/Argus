ARG PLAN9_BASE
FROM ${PLAN9_BASE}
COPY workload.py /usr/local/lib/plan9-b08.py
COPY plan9-b08.service /etc/systemd/system/plan9-b08.service
COPY plan9-b08-load.service /etc/systemd/system/plan9-b08-load.service
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    systemctl enable plan9-b08.service && \
    mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nSystemMaxUse=32M\nRuntimeMaxUse=16M\n' > /etc/systemd/journald.conf.d/plan9.conf
