ARG BASE_IMAGE=argus-plan9-host:b01-20261026
FROM ${BASE_IMAGE}
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service
COPY app-binary /usr/local/bin/plan9-http-app
COPY load.py /usr/local/lib/plan9-load.py
COPY systemd/ /etc/systemd/system/
RUN chmod 755 /usr/local/bin/plan9-http-app && sed -i 's/\r$//' /usr/local/lib/plan9-load.py /etc/systemd/system/plan9-*.service
