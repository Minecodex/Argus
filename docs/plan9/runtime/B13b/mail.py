"""Owned TLS SMTP inbox: bounded receipts, no forwarding, controlled 451 retry."""
import base64
from datetime import datetime, timezone
from email.parser import BytesParser
from email.policy import default
import hashlib
import json
from pathlib import Path
import socketserver
import ssl
import threading

ROOT = Path('/var/lib/plan9/mail')
LOCK = threading.Lock()
CONFIG = Path('/etc/plan9/receiver')


def receipt(kind, body=b'', **extra):
    ROOT.mkdir(parents=True, exist_ok=True, mode=0o700)
    value = {'at': datetime.now(timezone.utc).isoformat(), 'kind': kind, **extra}
    if body:
        message = BytesParser(policy=default).parsebytes(body)
        value.update(batch=message.get('X-Argus-Notification-Batch'), subject=str(message.get('Subject', '')),
                     sha256=hashlib.sha256(body).hexdigest(), bytes=len(body), body=body.decode('utf-8', 'replace'))
    with (ROOT / 'receipts.jsonl').open('a', encoding='utf-8') as stream:
        stream.write(json.dumps(value, ensure_ascii=False) + '\n')


class Handler(socketserver.StreamRequestHandler):
    def reply(self, value):
        self.wfile.write((value + '\r\n').encode())
        self.wfile.flush()

    def handle(self):
        self.connection.settimeout(20)
        self.reply('220 B13b owned TLS inbox')
        authorized = False
        recipient = False
        for _ in range(64):
            line = self.rfile.readline(4097)
            if not line or len(line) > 4096:
                return
            verb, _, argument = line.decode('utf-8', 'replace').strip().partition(' ')
            verb = verb.upper()
            if verb in ['EHLO', 'HELO']:
                self.reply('250-B13b owned inbox\r\n250-AUTH PLAIN\r\n250 SIZE 131072')
            elif verb == 'AUTH':
                settings = json.loads((CONFIG / 'auth.json').read_text())
                try:
                    _, user, password = base64.b64decode(argument.split()[-1], validate=True).decode().split('\x00')
                    authorized = user == settings['username'] and password == settings['password']
                except (ValueError, UnicodeError):
                    authorized = False
                self.reply('235 authenticated' if authorized else '535 authentication rejected')
            elif verb == 'QUIT':
                self.reply('221 bye')
                return
            elif not authorized:
                self.reply('530 authentication required')
            elif verb == 'MAIL':
                self.reply('250 sender accepted')
            elif verb == 'RCPT':
                recipient = argument.lower() == 'to:<b13b@plan9.invalid>'
                self.reply('250 local recipient accepted' if recipient else '550 not a local recipient')
            elif verb == 'DATA' and recipient:
                self.reply('354 end with dot')
                chunks = []
                total = 0
                while True:
                    part = self.rfile.readline(131073)
                    if not part:
                        return
                    if part == b'.\r\n':
                        break
                    total += len(part)
                    if total > 131072:
                        self.reply('552 message too large')
                        return
                    chunks.append(part[1:] if part.startswith(b'..') else part)
                body = b''.join(chunks)
                with LOCK:
                    mode_file = ROOT / 'mode.json'
                    mode = json.loads(mode_file.read_text()) if mode_file.exists() else {'mode': 'accept'}
                    if mode['mode'] == 'temporary_once' and mode.get('remaining', 1) > 0:
                        mode['remaining'] = mode.get('remaining', 1) - 1
                        mode_file.write_text(json.dumps(mode))
                        receipt('temporary_rejection', body, code=451)
                        self.reply('451 controlled temporary local failure')
                    elif mode['mode'] == 'permanent':
                        receipt('permanent_rejection', body, code=550)
                        self.reply('550 controlled permanent local failure')
                    else:
                        receipt('accepted', body, code=250)
                        self.reply('250 accepted into owned local inbox')
            elif verb in ['RSET', 'NOOP']:
                self.reply('250 ok')
            else:
                self.reply('500 unsupported command')


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True

    def get_request(self):
        connection, address = super().get_request()
        connection.settimeout(10)
        try:
            return self.context.wrap_socket(connection, server_side=True), address
        except (ssl.SSLError, OSError):
            connection.close()
            raise


if __name__ == '__main__':
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.minimum_version = ssl.TLSVersion.TLSv1_2
    context.load_cert_chain(CONFIG / 'server.crt', CONFIG / 'server.key')
    with Server(('0.0.0.0', 1465), Handler) as server:
        server.context = context
        server.serve_forever()
