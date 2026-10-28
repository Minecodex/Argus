"""Generate real isolated test certificates; never change the system clock."""
from datetime import datetime, timedelta, timezone
from pathlib import Path
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID
import json


def generate(directory: Path):
    if (directory/'manifest.json').exists():return
    directory.mkdir(parents=True,exist_ok=True)
    now=datetime.now(timezone.utc)
    ca_key=rsa.generate_private_key(public_exponent=65537,key_size=2048)
    issuer=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'Plan9 B02b isolated CA')])
    ca=(x509.CertificateBuilder().subject_name(issuer).issuer_name(issuer).public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number()).not_valid_before(now-timedelta(minutes=5))
        .not_valid_after(now+timedelta(days=3650)).add_extension(x509.BasicConstraints(ca=True,path_length=0),critical=True)
        .sign(ca_key,hashes.SHA256()))
    def key_bytes(key):return key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption())
    (directory/'ca.key').write_bytes(key_bytes(ca_key))
    (directory/'ca.crt').write_bytes(ca.public_bytes(serialization.Encoding.PEM))
    manifest={'created_at':now.isoformat(),'clock_modified':False,'targets':{}}
    for host in ['target-a','target-b']:
        folder=directory/host;folder.mkdir(exist_ok=True)
        key=rsa.generate_private_key(public_exponent=65537,key_size=2048)
        (folder/'server.key').write_bytes(key_bytes(key))
        manifest['targets'][host]={}
        for name,start,end in [('normal',now-timedelta(minutes=5),now+timedelta(days=90)),
                               ('short',now-timedelta(minutes=5),now+timedelta(days=2)),
                               ('expired',now-timedelta(days=2),now-timedelta(days=1))]:
            cert=(x509.CertificateBuilder().subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,host)]))
                  .issuer_name(issuer).public_key(key.public_key()).serial_number(x509.random_serial_number())
                  .not_valid_before(start).not_valid_after(end)
                  .add_extension(x509.SubjectAlternativeName([x509.DNSName(host)]),critical=False)
                  .add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]),critical=False)
                  .sign(ca_key,hashes.SHA256()))
            (folder/(name+'.crt')).write_bytes(cert.public_bytes(serialization.Encoding.PEM))
            manifest['targets'][host][name]={'not_before':start.isoformat(),'not_after':end.isoformat(),
                'sha256':cert.fingerprint(hashes.SHA256()).hex()}
    (directory/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf-8',newline='\n')
