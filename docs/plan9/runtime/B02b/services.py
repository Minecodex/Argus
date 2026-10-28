#!/usr/bin/python3
"""Actual HTTP/HTTPS and TCP servers. Telemetry is collected externally."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socketserver
import ssl
import sys
import time


class HTTP(BaseHTTPRequestHandler):
    def do_GET(self):
        failed=Path('/var/lib/plan9/http-fault').exists()
        body=json.dumps({'node':os.environ['PLAN9_NODE'],'status':'unavailable' if failed else 'ok','time':time.time()}).encode()
        self.send_response(503 if failed else 200)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(body)))
        self.end_headers()
        try:self.wfile.write(body)
        except (BrokenPipeError,ConnectionResetError):pass

    def log_message(self,*_):pass


class Echo(socketserver.BaseRequestHandler):
    def handle(self):
        self.request.settimeout(2)
        try:
            data=self.request.recv(1024)
            if data:self.request.sendall(b'B02b '+data)
        except (OSError,TimeoutError):pass


class TCP(socketserver.ThreadingTCPServer):
    allow_reuse_address=True
    daemon_threads=True


if __name__=='__main__':
    mode=sys.argv[1]
    if mode=='tcp':
        with TCP(('0.0.0.0',9000),Echo) as server:server.serve_forever()
    elif mode in ('http','tls'):
        server=ThreadingHTTPServer(('0.0.0.0',8080 if mode=='http' else 8443),HTTP)
        if mode=='tls':
            variant=Path('/var/lib/plan9/tls-variant').read_text().strip()
            assert variant in ('normal','short','expired')
            context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.load_cert_chain('/pki/'+variant+'.crt','/pki/server.key')
            server.socket=context.wrap_socket(server.socket,server_side=True)
        server.serve_forever()
    else:raise ValueError('unsupported server')
