#!/usr/bin/python3
"""Real bounded OS/HTTP work; contains no telemetry producer or Argus writes."""
import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import sys
import time
import urllib.request

ROOT = Path('/var/lib/plan9')
ROOT.mkdir(parents=True, exist_ok=True)


def emit(**fields):
    print(json.dumps({'time': time.time(), 'run': os.getenv('PLAN9_RUN'), **fields}), flush=True)


class Handler(http.server.BaseHTTPRequestHandler):
    count = 0

    def do_GET(self):
        Handler.count += 1
        payload = b'x' * 262144 if self.path == '/payload' else json.dumps(
            {'hostname': os.uname().nodename, 'requests': Handler.count, 'time': time.time()}
        ).encode()
        self.send_response(200)
        self.send_header('Content-Length', str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt, *args):
        if Handler.count % 30 == 0:
            emit(action='http_requests', total=Handler.count)


def run(mode, seconds):
    end = time.monotonic() + seconds
    if mode == 'serve':
        if (ROOT / 'service-fault').exists():
            emit(action='controlled_service_failure')
            return 42
        http.server.ThreadingHTTPServer(('0.0.0.0', 8080), Handler).serve_forever()
    elif mode == 'cpu':
        payload = b'x' * 1048576
        count = 0
        while time.monotonic() < end:
            hashlib.sha256(payload).digest()
            count += 1
        emit(action='cpu_complete', hashes=count)
    elif mode == 'memory':
        data = bytearray(160 * 1024 * 1024)
        for i in range(0, len(data), 4096):
            data[i] = 1
        emit(action='memory_allocated', bytes=len(data))
        while time.monotonic() < end:
            time.sleep(1)
    elif mode == 'disk':
        payload = os.urandom(1048576)
        count = 0
        path = ROOT / 'bounded-64m.bin'
        with path.open('w+b') as stream:
            while time.monotonic() < end:
                stream.seek((count % 64) * len(payload))
                stream.write(payload)
                stream.flush()
                os.fsync(stream.fileno())
                stream.seek(0)
                stream.read(min((count + 1), 64) * len(payload))
                count += 1
                time.sleep(0.2)
        emit(action='disk_complete', writes=count, max_bytes=64 * len(payload))
    elif mode in ('traffic', 'network'):
        peer = os.environ.get('PLAN9_PEER', '127.0.0.1')
        url = 'http://' + peer + ':8080/' + ('payload' if mode == 'network' else 'health')
        success = failure = size = 0
        while time.monotonic() < end:
            try:
                with urllib.request.urlopen(url, timeout=3) as response:
                    size += len(response.read())
                success += 1
            except Exception:
                failure += 1
            if (success + failure) % 30 == 0:
                emit(action=mode, success=success, failure=failure, bytes=size)
            time.sleep(0.1 if mode == 'network' else 2)
        emit(action=mode + '_complete', success=success, failure=failure, bytes=size)
    return 0


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['serve', 'cpu', 'memory', 'disk', 'traffic', 'network'])
    parser.add_argument('--seconds', type=int, default=120)
    args = parser.parse_args()
    if not 1 <= args.seconds <= (86400 if args.mode == 'traffic' else 300):
        parser.error('duration exceeds the bounded workload budget')
    sys.exit(run(args.mode, args.seconds))
