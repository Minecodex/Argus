"""Authenticated TLS Helm repository; serves actual packaged Charts."""
import base64
import hmac
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import ssl

account = json.loads(Path('/private/source-account.json').read_text())
expected = 'Basic '+base64.b64encode((account['username']+':'+account['password']).encode()).decode()


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory='/charts', **kwargs)

    def log_message(self, _format, *_args):
        pass

    def do_GET(self):
        if not hmac.compare_digest(self.headers.get('Authorization',''), expected):
            self.send_response(401)
            self.send_header('WWW-Authenticate','Basic realm="plan9-b10c"')
            self.end_headers()
            return
        print(json.dumps({'run':os.environ['PLAN9_RUN'],'request_path':self.path}), flush=True)
        super().do_GET()


server = ThreadingHTTPServer(('0.0.0.0',8443), Handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version = ssl.TLSVersion.TLSv1_2
context.load_cert_chain('/private/server.crt','/private/server.key')
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
