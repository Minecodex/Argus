"""Small real RESP client; credentials are read only from the private mount."""
import json
from pathlib import Path
import socket

class Redis:
    def __init__(self,role='app'):
        self.sock=socket.create_connection(('redis',6379),5);self.sock.settimeout(5);self.reader=self.sock.makefile('rb')
        credentials=json.loads(Path('/root/plan9-redis.json').read_text())
        self.command('AUTH',role,credentials[role])
    def response(self):
        line=self.reader.readline();kind=line[:1];value=line[1:-2]
        if kind==b'-':raise RuntimeError(value.decode())
        if kind==b'+':return value.decode()
        if kind==b':':return int(value)
        if kind==b'$':
            size=int(value)
            if size<0:return None
            assert size<=1048576
            data=self.reader.read(size);assert self.reader.read(2)==b'\r\n';return data
        if kind==b'*':return [self.response() for _ in range(int(value))]
        raise RuntimeError('Unexpected Redis protocol response')
    def command(self,*args):
        parts=[item if isinstance(item,bytes) else str(item).encode() for item in args]
        self.sock.sendall(b'*'+str(len(parts)).encode()+b'\r\n'+b''.join(b'$'+str(len(item)).encode()+b'\r\n'+item+b'\r\n' for item in parts))
        return self.response()
    def close(self):self.reader.close();self.sock.close()
