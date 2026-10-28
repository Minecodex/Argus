ARG BASE_IMAGE=argus-plan9-host:b01-20261026
FROM ${BASE_IMAGE}
RUN systemctl disable plan9-web.service plan9-anchor.service plan9-traffic.service && \
    apt-get -o Acquire::Retries=3 update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends python3-pymysql python3-psycopg2 python3-cryptography && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
COPY client.py /usr/local/lib/plan9-db-client.py
RUN sed -i 's/\r$//' /usr/local/lib/plan9-db-client.py
