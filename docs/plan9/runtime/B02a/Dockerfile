ARG BASE_IMAGE=argus-plan9-host:b01-20261026
FROM ${BASE_IMAGE}
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    apt-get -o Acquire::Retries=3 update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends nginx apache2 haproxy && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    a2enmod proxy proxy_http status && a2dissite 000-default && \
    rm -f /etc/nginx/sites-enabled/default && \
    printf 'Listen 8082\n' > /etc/apache2/ports.conf
COPY config/nginx.conf /etc/nginx/sites-enabled/plan9
COPY config/apache.conf /etc/apache2/sites-enabled/plan9.conf
COPY config/haproxy.cfg /etc/haproxy/haproxy.cfg
COPY services.py /usr/local/bin/plan9-web-lab
COPY bootstrap.sh /usr/local/bin/plan9-web-bootstrap
COPY systemd/ /etc/systemd/system/
RUN sed -i 's/\r$//' /usr/local/bin/plan9-web-lab /usr/local/bin/plan9-web-bootstrap && \
    chmod 755 /usr/local/bin/plan9-web-lab /usr/local/bin/plan9-web-bootstrap && \
    chmod 644 /etc/systemd/system/plan9-*.service && \
    mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nSystemMaxUse=32M\nRuntimeMaxUse=16M\n' > /etc/systemd/journald.conf.d/plan9.conf && \
    systemctl enable nginx apache2 haproxy plan9-backend plan9-proxy-backend plan9-web-traffic && \
    nginx -t && apache2ctl -t && haproxy -c -f /etc/haproxy/haproxy.cfg
ENTRYPOINT ["/usr/local/bin/plan9-web-bootstrap"]
