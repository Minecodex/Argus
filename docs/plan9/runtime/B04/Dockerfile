ARG BASE_IMAGE=argus-plan9-host:b01-20261026
FROM ${BASE_IMAGE}
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service
COPY client.py /usr/local/lib/plan9-cache-client.py
RUN sed -i 's/\r$//' /usr/local/lib/plan9-cache-client.py
