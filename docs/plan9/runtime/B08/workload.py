"""Real HTTP results and application/system logs on the owned Linux host."""
import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import re
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit, parse_qs
from urllib.request import urlopen

ROOT = Path('/var/lib/plan9/b08')
ADDR = 'http://127.0.0.1:18080'
RUN = os.environ.get('PLAN9_RUN', 'plan9-b08')
STREAMS = ('rename', 'copy')


def append(path, body):
    if not path.exists() or path.stat().st_size < 4 * 1024 * 1024:
        with path.open('a', encoding='utf-8') as stream:
            stream.write(body + '\n')


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        return

    def send(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        url = urlsplit(self.path)
        if url.path == '/health':
            self.send(200, {'status': 'ok'})
            return
        args = parse_qs(url.query)
        label = args.get('label', ['live'])[0]
        stream = args.get('stream', ['rename'])[0]
        if url.path not in ('/orders', '/error') or stream not in STREAMS or not re.fullmatch(r'[a-z][a-z0-9-]{0,50}', label):
            self.send(400, {'error': 'invalid request'})
            return
        error = url.path == '/error'
        counter = ROOT / 'sequence'
        sequence = (int(counter.read_text()) if counter.exists() else 0) + 1
        temp = counter.with_suffix('.tmp')
        temp.write_text(str(sequence))
        temp.replace(counter)
        marker = f'{RUN}-{label}-{sequence:06d}'
        status = 503 if error else 200
        event = {
            'message': marker, 'run_id': RUN, 'sequence': sequence,
            'timestamp': datetime.now(timezone.utc).isoformat(), 'status': status,
            'operation': url.path, 'severity': 'ERROR' if error else 'INFO',
        }
        append(ROOT / 'json' / (stream + '.log'), json.dumps(event))
        if error:
            append(ROOT / 'stack' / 'errors.log',
                   f'ERROR {marker}\n  at orders.validate_request\n  caused by controlled dependency refusal')
        print(json.dumps({**event, 'message': marker + '-journal', 'unit': 'plan9-b08.service'}), flush=True)
        self.send(status, event)


def request(label, stream='rename', error=False):
    try:
        response = urlopen(ADDR + ('/error' if error else '/orders') + '?' + urlencode({'label': label, 'stream': stream}), timeout=3)
    except HTTPError as exc:
        response = exc
    with response:
        data = json.load(response)
        assert response.status == data['status']
        return data


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['serve', 'request', 'load'])
    parser.add_argument('--label', default='live')
    parser.add_argument('--stream', default='rename', choices=STREAMS)
    parser.add_argument('--error', action='store_true')
    args = parser.parse_args()
    if args.action == 'serve':
        for folder in ('json', 'stack'):
            (ROOT / folder).mkdir(parents=True, exist_ok=True)
        if (ROOT / 'fail-start').exists():
            raise SystemExit('B08 controlled service startup failure')
        HTTPServer(('127.0.0.1', 18080), Handler).serve_forever()
    elif args.action == 'request':
        print(json.dumps(request(args.label, args.stream, args.error)))
    else:
        deadline = time.monotonic() + 8 * 3600
        for i in range(1920):
            if time.monotonic() >= deadline:
                break
            try:
                request('live-error' if i % 8 == 0 else 'live', error=i % 8 == 0)
            except URLError:
                pass
            time.sleep(15)
