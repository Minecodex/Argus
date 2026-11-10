"""Low-rate real DNS/API/scale activity, bounded to one review session."""
import json
import os
from pathlib import Path
import socket
import ssl
import time
from urllib.request import Request, urlopen

BASE = Path('/var/run/secrets/kubernetes.io/serviceaccount')
CONTEXT = ssl.create_default_context(cafile=str(BASE / 'ca.crt'))
URL = 'https://kubernetes.default.svc/apis/apps/v1/namespaces/b09b-components/deployments/metric-workload/scale'


def request(replicas=None):
    token = (BASE / 'token').read_text().strip()
    data = None if replicas is None else json.dumps({'spec': {'replicas': replicas}}).encode()
    headers = {'Authorization': 'Bearer ' + token}
    if data is not None:
        headers['Content-Type'] = 'application/merge-patch+json'
    req = Request(URL, data=data, method='GET' if data is None else 'PATCH', headers=headers)
    with urlopen(req, context=CONTEXT, timeout=5) as response:
        return json.load(response)['spec']['replicas']


def main():
    deadline = float(os.environ['B09B_ACTIVITY_UNTIL'])
    turn = 0
    try:
        while time.time() < deadline:
            try:
                # CoreDNS cache hits and ordinary service queries, never synthetic metrics.
                for _ in range(3):
                    socket.getaddrinfo('kubernetes.default.svc.cluster.local', 443)
                if turn % 3 == 0:
                    request(1 + (turn // 3) % 2)
                else:
                    request()
                print(json.dumps({'event': 'real_dns_api_activity', 'iteration': turn}), flush=True)
            except Exception as error:
                print(json.dumps({'event': 'activity_error', 'type': type(error).__name__}), flush=True)
            turn += 1
            time.sleep(10)
    finally:
        request(1)
    # Stay idle so the Deployment restart policy cannot restart the activity
    # budget. An absolute deadline also survives an earlier container restart.
    while True:
        time.sleep(30)


if __name__ == '__main__':
    main()
