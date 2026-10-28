#!/usr/bin/python3
"""Real HTTP application and bounded clients; no telemetry generator."""
import argparse
from concurrent.futures import ThreadPoolExecutor
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import time
from urllib.parse import urlparse, parse_qs
from http.client import HTTPConnection


class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == '/hold':
            delay = min(15, max(0, float(parse_qs(parsed.query).get('seconds', ['10'])[0])))
            time.sleep(delay)
        body = json.dumps({'node': os.environ.get('PLAN9_NODE'), 'path': parsed.path,
                           'time': time.time(), 'payload': 'web-request-' * 100}).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def log_message(self, *_):
        pass


def request(url):
    parsed = urlparse(url)
    assert parsed.scheme == 'http'
    connection = HTTPConnection(parsed.hostname, parsed.port, timeout=22)
    try:
        connection.request('GET', parsed.path + ('?' + parsed.query if parsed.query else ''))
        response = connection.getresponse()
        return response.status, len(response.read(32768))
    except (OSError, TimeoutError):
        return 0, 0
    finally:
        connection.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['serve', 'traffic', 'hold'])
    parser.add_argument('--seconds', type=int, default=90)
    parser.add_argument('--port', type=int, choices=[8081, 8082, 8083, 9000, 9001], default=9000)
    parser.add_argument('--target', default='127.0.0.1')
    args = parser.parse_args()
    if args.mode == 'serve':
        ThreadingHTTPServer(('0.0.0.0', args.port), Handler).serve_forever()
        return
    assert 1 <= args.seconds <= (86400 if args.mode == 'traffic' else 120)
    remaining = args.seconds
    if args.mode == 'traffic' and os.environ.get('PLAN9_TRAFFIC_DEADLINE'):
        remaining = min(remaining, max(0, float(os.environ['PLAN9_TRAFFIC_DEADLINE']) - time.time()))
    end = time.monotonic() + remaining
    counts, received = {}, 0
    with ThreadPoolExecutor(max_workers=12) as workers:
        while time.monotonic() < end:
            urls = ([f'http://{args.target}:{port}/data' for port in [8081, 8082, 8083]]
                    if args.mode == 'traffic' else [f'http://{args.target}:{args.port}/hold?seconds=10'] * 12)
            for status, length in workers.map(request, urls):
                counts[str(status)] = counts.get(str(status), 0) + 1
                received += length
            if args.mode == 'traffic':
                time.sleep(2)
    print(json.dumps({'mode': args.mode, 'statuses': counts, 'received_bytes': received,
                      'seconds': args.seconds}), flush=True)


if __name__ == '__main__':
    main()
