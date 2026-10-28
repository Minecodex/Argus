ARG BASE_IMAGE=argus-plan9-host:b01-20261026
FROM ${BASE_IMAGE}
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    apt-get -o Acquire::Retries=3 update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends supervisor dnsmasq-base dnsutils iptables && \
    systemctl disable supervisor.service && apt-get clean && rm -rf /var/lib/apt/lists/*
COPY services.py /usr/local/bin/plan9-probe-services
COPY bootstrap.sh /usr/local/bin/plan9-probe-bootstrap
COPY supervisord.conf /etc/plan9-supervisord.conf
RUN sed -i 's/\r$//' /usr/local/bin/plan9-probe-services /usr/local/bin/plan9-probe-bootstrap && \
    chmod 755 /usr/local/bin/plan9-probe-services /usr/local/bin/plan9-probe-bootstrap
ENTRYPOINT ["/usr/local/bin/plan9-probe-bootstrap"]
