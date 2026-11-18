"""Own receiver certificate and ordinary notification credential/channel setup."""
from datetime import datetime, timedelta, timezone
import ipaddress
import json
import secrets
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID
from manage import Run, write_json, now
from integration import Batch


def configure(batch):
    run, api = batch.run, batch.api
    private = run.private / 'receiver'
    private.mkdir(exist_ok=True)
    address = batch.host_specs['reports']['address']
    if not (private / 'ca.pem').exists():
        clock = datetime.now(timezone.utc)
        key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, run.id + ' local notification CA')])
        ca = x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(clock-timedelta(minutes=5)).not_valid_after(clock+timedelta(days=7)).add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True).add_extension(x509.KeyUsage(False, False, False, False, False, True, True, False, False), critical=True).sign(key, hashes.SHA256())
        leaf_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        leaf = x509.CertificateBuilder().subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, 'B13b owned TLS inbox')])).issuer_name(name).public_key(leaf_key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(clock-timedelta(minutes=5)).not_valid_after(clock+timedelta(days=7)).add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True).add_extension(x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address(address))]), critical=False).add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), critical=False).sign(key, hashes.SHA256())
        (private / 'ca.pem').write_bytes(ca.public_bytes(serialization.Encoding.PEM))
        (private / 'server.crt').write_bytes(leaf.public_bytes(serialization.Encoding.PEM)+ca.public_bytes(serialization.Encoding.PEM))
        (private / 'server.key').write_bytes(leaf_key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))
        write_json(private / 'auth.json', {'username': 'plan9-b13b', 'password': secrets.token_urlsafe(32)})
        # The issuer key is not retained or sent to Argus. Only the public CA
        # goes into the notification credential; server key stays in this mount.
    if run.shell('reports', 'systemctl is-active plan9-b13b-mail.service || true\n') != 'active':
        run.shell('reports', 'systemd-run --collect --unit=plan9-b13b-mail --property=RuntimeMaxSec=28800 --property=MemoryMax=96M --property=CPUQuota=5% /usr/bin/python3 /usr/local/lib/plan9-b13b/mail.py\n')
    state = batch.map.setdefault('notification', {})
    auth = json.loads((private / 'auth.json').read_text(encoding='utf-8'))
    if 'secret_id' not in state:
        credential = {'smtp_host': address, 'smtp_port': 1465, 'security': 'tls', **auth, 'from': 'alerts@plan9.invalid', 'to': ['b13b@plan9.invalid'],
                      'ca_pem': (private / 'ca.pem').read_text(encoding='utf-8')}
        row = api.call('POST', '/enterprise/secrets', {'name': run.id + '-local-smtp', 'type': 'notification', 'value': json.dumps(credential)}, key='owned-smtp-secret', expected=201)
        state['secret_id'] = row['id']
        batch.save()
    if 'credential_id' not in state:
        row = api.call('POST', '/enterprise/credentials', {'name': run.id + '-local-smtp', 'protocol': 'notification', 'secret_id': state['secret_id']}, key='owned-smtp-credential', expected=201)
        state['credential_id'] = row['id']
        batch.save()
    if 'channel_id' not in state:
        api.confirm(api.call('POST', '/alerts/configurations/preview', {'kind': 'channel', 'operation': 'create', 'name': 'B13b 本地 TLS 收件器',
                        'spec': {'enabled': True, 'provider': 'smtp', 'credential_id': state['credential_id'], 'display_address': 'B13b owned inbox only',
                                 'default_for_new_policies': False, 'mention_ids': [], 'mention_all': False, 'template': ''}}, key='owned-smtp-channel', expected=202))
        row = next(x for x in api.call('GET', '/alerts/configurations?kind=channel') if x['name'] == 'B13b 本地 TLS 收件器')
        state['channel_id'] = row['id']
        batch.save()
    run.save('notification-receiver', {'at': now(), 'address': address, 'port': 1465, 'tls_hostname_verification': True,
                                      'public_ca_only_in_credential': True, 'global_trust_unchanged': True, 'external_recipient': False,
                                      'recipient': 'b13b@plan9.invalid', 'channel_id': state['channel_id']})
    print('Owned TLS SMTP receiver and normal scoped credential/channel configured', flush=True)


if __name__ == '__main__':
    run = Run()
    api = run.api()
    api.login()
    configure(Batch(run, api))
