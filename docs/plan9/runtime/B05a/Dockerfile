ARG BASE_IMAGE=argus-plan9-host:b01-20261026
FROM ${BASE_IMAGE}
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    apt-get -o Acquire::Retries=3 update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends python3-pika && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
COPY client.py /usr/local/lib/plan9-mq-client.py
COPY systemd/ /etc/systemd/system/
RUN sed -i 's/\r$//' /usr/local/lib/plan9-mq-client.py /etc/systemd/system/plan9-mq-*.service
