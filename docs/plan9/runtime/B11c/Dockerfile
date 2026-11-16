ARG PLAN9_BASE=argus-plan9-host:b11c-base-20261115
FROM ${PLAN9_BASE}
RUN apt-get -o Acquire::Retries=3 update && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=3 install -y --no-install-recommends iptables && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
COPY service.py /usr/local/lib/plan9-b11c.py
COPY plan9-b11c.service /etc/systemd/system/plan9-b11c.service
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    systemctl enable plan9-b11c.service && mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nSystemMaxUse=32M\nRuntimeMaxUse=16M\n' > /etc/systemd/journald.conf.d/plan9.conf
