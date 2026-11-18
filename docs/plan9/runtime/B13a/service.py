"""Bounded real RESP and Memcached requests; no synthetic telemetry or DB writes."""
import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import time
from urllib.request import urlopen

APP = os.environ.get('PLAN9_APP', 'orders')
COUNT = 0


class Redis:
    def __init__(self):
        self.sock = socket.create_connection(('redis', 6379), 4)
        self.reader = self.sock.makefile('rb')
        credentials = json.loads(Path('/root/plan9-redis.json').read_text())
        self.command('AUTH', 'app', credentials['app'])

    def response(self):
        line = self.reader.readline()
        kind, value = line[:1], line[1:-2]
        if kind == b'-':
            raise RuntimeError(value.decode())
        if kind == b'+':
            return value.decode()
        if kind == b':':
            return int(value)
        if kind == b'$':
            size = int(value)
            if size < 0:
                return None
            assert size <= 1048576
            data = self.reader.read(size)
            assert self.reader.read(2) == b'\r\n'
            return data
        raise RuntimeError('Unexpected Redis response')

    def command(self, *args):
        parts = [str(x).encode() for x in args]
        self.sock.sendall(b'*' + str(len(parts)).encode() + b'\r\n' + b''.join(b'$' + str(len(x)).encode() + b'\r\n' + x + b'\r\n' for x in parts))
        return self.response()

    def close(self):
        self.reader.close()
        self.sock.close()


def caches():
    client = Redis()
    try:
        assert client.command('PING') == 'PONG'
        key = 'p9:b13a:' + APP + ':' + str(COUNT % 32)
        hit = client.command('GET', key) is not None
        assert client.command('SET', key, APP, 'EX', 600) == 'OK'
    finally:
        client.close()
    result = {'redis_endpoint': 'redis:6379', 'redis_hit': hit}
    if APP == 'reports':
        with socket.create_connection(('memcached', 11211), 4) as sock:
            reader = sock.makefile('rb')
            sock.sendall(b'set p9-b13a-report 0 600 7\r\nreports\r\n')
            assert reader.readline() == b'STORED\r\n'
            sock.sendall(b'get p9-b13a-report\r\n')
            assert reader.readline().startswith(b'VALUE ')
            assert reader.readline() == b'reports\r\n'
            assert reader.readline() == b'END\r\n'
        result['memcached_endpoint'] = 'memcached:11211'
    return result


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        global COUNT
        try:
            COUNT += 1
            result = {'app': APP, 'environment': 'production' if APP == 'orders' else 'staging', 'run': os.environ['PLAN9_RUN'],
                      'at': datetime.now(timezone.utc).isoformat(), 'requests': COUNT, **caches()}
            raw = json.dumps(result).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(raw)
            if not self.path.startswith('/health'):
                print(raw.decode(), flush=True)
        except (OSError, AssertionError, RuntimeError) as error:
            self.send_response(503)
            self.end_headers()
            self.wfile.write(type(error).__name__.encode())


def serve():
    ThreadingHTTPServer(('0.0.0.0', 18085), Handler).serve_forever()


def traffic():
    deadline = time.monotonic() + 28800
    while time.monotonic() < deadline:
        try:
            with urlopen('http://127.0.0.1:18085/request', timeout=5) as response:
                response.read(4096)
        except OSError as error:
            print(type(error).__name__, flush=True)
        time.sleep(5)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['serve', 'traffic'])
    globals()[parser.parse_args().action]()
