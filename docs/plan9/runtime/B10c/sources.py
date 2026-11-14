"""Immutable workload image, native TLS Helm repository and OCI registry."""
from datetime import datetime, timedelta, timezone
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import secrets
import subprocess
import time

import bcrypt
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID
import requests

from infrastructure import HERE, BASE_IMAGE, execute


def source_private(run):
    return run.private/'sources'


def compose(run, *args):
    return execute(['docker','compose','--project-name',run.id,'--env-file',str(run.out/'compose.env'),
                    '--file',str(HERE/'compose.yaml'),*args], timeout=180)


def prepare(run):
    state_file = run.out/'source-state.json'
    if state_file.exists() and json.loads(state_file.read_text(encoding='utf-8')).get('prepared'):
        return
    source_private(run).mkdir(exist_ok=True)
    network = json.loads(execute(['docker','network','inspect','kind']))[0]
    subnet = next(ipaddress.ip_network(row['Subnet']) for row in network['IPAM']['Config'] if ':' not in row['Subnet'])
    occupied = {v['IPv4Address'].split('/')[0] for v in network['Containers'].values()}
    candidates = [str(subnet.network_address+i) for i in range(250,270) if str(subnet.network_address+i) not in occupied]
    assert len(candidates) >= 2
    addresses = {'repository':candidates[0],'registry':candidates[1]}
    tag = 'localhost:5001/argus/plan9-b10c-workload:'+run.id
    execute(['docker','tag',BASE_IMAGE,tag])
    (run.out/'workload-image-push.log').write_text(execute(['docker','push',tag]), encoding='utf-8')
    image = json.loads(execute(['docker','image','inspect',tag]))[0]
    digest = next(d.split('@')[1] for d in image['RepoDigests'] if d.startswith('localhost:5001/argus/plan9-b10c-workload@'))
    run.save('source-state', {'run_id':run.id,'addresses':addresses,'source_image':tag+'@'+digest,
                              'workload_image':tag.replace('localhost','host.docker.internal')+'@'+digest,
                              'image_id':image['Id'],'shared_image_registry':'localhost:5001 (borrowed cache)'})
    account = {'username':'b10c-source','password':secrets.token_urlsafe(32)}
    (source_private(run)/'source-account.json').write_text(json.dumps(account), encoding='utf-8')
    (source_private(run)/'htpasswd').write_bytes(account['username'].encode()+b':'+bcrypt.hashpw(account['password'].encode(),bcrypt.gensalt())+b'\n')
    now = datetime.now(timezone.utc)
    ca_key = rsa.generate_private_key(public_exponent=65537,key_size=2048)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, run.id+'-sources')])
    ca = x509.CertificateBuilder().subject_name(subject).issuer_name(subject).public_key(ca_key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now-timedelta(minutes=5)).not_valid_after(now+timedelta(days=8)).add_extension(x509.BasicConstraints(ca=True,path_length=0),critical=True).sign(ca_key,hashes.SHA256())
    key = rsa.generate_private_key(public_exponent=65537,key_size=2048)
    cert = x509.CertificateBuilder().subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,run.id+'-endpoint')])).issuer_name(subject).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now-timedelta(minutes=5)).not_valid_after(now+timedelta(days=7)).add_extension(x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address(v)) for v in [*addresses.values(),'127.0.0.1']]+[x509.DNSName('localhost')]),critical=False).add_extension(x509.BasicConstraints(ca=False,path_length=None),critical=True).sign(ca_key,hashes.SHA256())
    (source_private(run)/'source-ca.pem').write_bytes(ca.public_bytes(serialization.Encoding.PEM))
    (source_private(run)/'server.crt').write_bytes(cert.public_bytes(serialization.Encoding.PEM))
    (source_private(run)/'server.key').write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
    from chart import package
    package(run)
    env = {'B10C_RUN':run.id,'B10C_SOURCE_IMAGE':tag+'@'+digest,'B10C_PACKAGE_DIR':str(HERE).replace('\\','/'),
           'B10C_PRIVATE_DIR':str(source_private(run)).replace('\\','/'),'B10C_CHARTS_DIR':str(run.out/'charts').replace('\\','/'),
           'B10C_REPOSITORY_IP':addresses['repository'],'B10C_REGISTRY_IP':addresses['registry']}
    (run.out/'compose.env').write_text('\n'.join(k+'='+v for k,v in env.items())+'\n',encoding='utf-8')
    (run.out/'compose-rendered.yaml').write_text(compose(run,'config'),encoding='utf-8')
    state = json.loads(state_file.read_text(encoding='utf-8'))
    state['prepared'] = True
    run.save('source-state',state)


def execute_stage(run, _action):
    prepare(run)
    compose(run,'up','-d','--pull','never')
    account = json.loads((source_private(run)/'source-account.json').read_text(encoding='utf-8'))
    http = requests.Session()
    http.trust_env = False
    http.verify = str(source_private(run)/'source-ca.pem')
    http.auth = (account['username'],account['password'])
    addresses = run.source_addresses()
    for url in ('https://127.0.0.1:23443/index.yaml','https://127.0.0.1:23500/v2/'):
        for _ in range(30):
            try:
                response = http.get(url,timeout=3)
                if response.status_code == 200:
                    break
            except requests.RequestException:
                pass
            time.sleep(1)
        else:
            raise RuntimeError('Owned TLS chart source did not become ready')
    registry = '127.0.0.1:23500'
    env = dict(os.environ)
    env['GOCACHE'] = os.environ.get('ARGUS_PLAN9_GOCACHE',str(run.out/'go-cache'))
    env['GOTMPDIR'] = str(run.out/'go-tmp')
    Path(env['GOTMPDIR']).mkdir(exist_ok=True)
    with (run.out/'oci-native-push.log').open('wb') as log:
        subprocess.run(['go','run',str(HERE/'publishoci'),'--repository',registry+'/plan9/b10c-web',
                        '--charts',str(run.out/'charts'),'--account',str(source_private(run)/'source-account.json'),
                        '--ca',str(source_private(run)/'source-ca.pem')],cwd=HERE.parents[3],env=env,
                       stdout=log,stderr=subprocess.STDOUT,check=True,timeout=300)
    manifest = http.get('https://'+registry+'/v2/plan9/b10c-web/manifests/0.1.0',
                        headers={'Accept':'application/vnd.oci.image.manifest.v1+json'},timeout=10)
    manifest.raise_for_status()
    run.save('helm-sources', {'repository':'https://'+addresses['repository']+':8443',
                             'oci':'oci://'+addresses['registry']+':5000/plan9/b10c-web','tls_verified':True,
                             'oci_manifest_digest':'sha256:'+hashlib.sha256(manifest.content).hexdigest(),
                             'chart_sha256':{v:hashlib.sha256((run.out/'charts'/('b10c-web-'+v+'.tgz')).read_bytes()).hexdigest() for v in ('0.1.0','0.2.0')}})
    print('Actual authenticated TLS Helm repository and OCI artifacts ready',flush=True)


def load_nodes(run):
    for node in run.get('nodes')['items']:
        name = node['metadata']['name']
        assert name.startswith(run.id+'-')
        execute(['docker','exec',name,'ctr','--namespace','k8s.io','images','pull','--plain-http',run.image],timeout=180)
        repository, digest = run.image.split('@')
        execute(['docker','exec',name,'ctr','--namespace','k8s.io','images','tag','--force',run.image,
                 repository.rsplit(':',1)[0]+'@'+digest])
    run.save('workload-image-nodes', {'image':run.image,'nodes':[n['metadata']['name'] for n in run.get('nodes')['items']]})
