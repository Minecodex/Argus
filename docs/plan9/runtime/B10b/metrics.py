"""Real metrics.k8s.io with verified kubelet and aggregation serving TLS."""
import base64,hashlib,json,time
import requests,yaml
from infrastructure import execute
def install(r):
 path=r.out/'metrics-server-v0.8.1.yaml'
 if not path.exists():
  url='https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.1/components.yaml?plan9='+r.id
  value=requests.get(url,timeout=60);value.raise_for_status();path.write_bytes(value.content)
 assert hashlib.sha256(path.read_bytes()).hexdigest()=='4a672c4891902573a3ff753cece5de1bf1f55dd053403dfec39df9d1636b7ff1'
 node=r.id+'-control-plane'
 addresses=r.get('node',node)['status']['addresses']
 expected_dns={node};expected_ips={x['address'] for x in addresses if x['type']=='InternalIP'}
 from cryptography import x509
 from cryptography.x509.oid import NameOID
 approved=[]
 for row in r.get('certificatesigningrequests')['items']:
  spec=row['spec']
  if spec.get('signerName')!='kubernetes.io/kubelet-serving' or row.get('status',{}).get('certificate'):continue
  assert spec['username']=='system:node:'+node
  csr=x509.load_pem_x509_csr(base64.b64decode(spec['request']))
  assert csr.subject.get_attributes_for_oid(NameOID.COMMON_NAME)[0].value=='system:node:'+node
  san=csr.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
  assert set(san.get_values_for_type(x509.DNSName))<=expected_dns
  assert {str(x) for x in san.get_values_for_type(x509.IPAddress)}<=expected_ips
  assert 'server auth' in spec['usages'] and csr.is_signature_valid
  r.kubectl('certificate','approve',row['metadata']['name']);approved.append(row['metadata']['name'])
 r.save('kubelet-serving-csr-approved',{'approved':approved,'node':node,'san_identity_verified':True})
 # Private signing material stays in this owned node; only the public CA leaves.
 root='/etc/kubernetes/pki/plan9-b10b-metrics'
 script='set -eu\nmkdir -p '+root+'\nif [ ! -f '+root+'/tls.crt ]; then\numask 077\nopenssl req -new -newkey rsa:2048 -nodes -keyout '+root+'/tls.key -out '+root+'/tls.csr -subj /CN=metrics-server.kube-system.svc\nprintf "subjectAltName=DNS:metrics-server.kube-system.svc,DNS:metrics-server.kube-system.svc.cluster.local\\nextendedKeyUsage=serverAuth\\n" > '+root+'/extensions.cnf\nopenssl x509 -req -in '+root+'/tls.csr -CA /etc/kubernetes/pki/ca.crt -CAkey /etc/kubernetes/pki/ca.key -CAcreateserial -out '+root+'/tls.crt -days 2 -extfile '+root+'/extensions.cnf\nfi'
 execute(['docker','exec',node,'sh','-ec',script],timeout=30)
 if not r.kubectl('get','secret','b10b-metrics-server-tls','-n','kube-system','--ignore-not-found','-o','name'):
  r.kubectl('create','secret','tls','b10b-metrics-server-tls','-n','kube-system','--cert='+root+'/tls.crt','--key='+root+'/tls.key')
 ca=execute(['docker','exec',node,'cat','/etc/kubernetes/pki/ca.crt'])
 r.apply([{'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':'b10b-kubelet-ca','namespace':'kube-system','labels':{'argus.plan9/run':r.id}},'data':{'ca.crt':ca}}])
 docs=[x for x in yaml.safe_load_all(path.read_text(encoding='utf-8')) if x]
 from images import stage
 for obj in docs:
  obj['metadata'].setdefault('labels',{})['argus.plan9/run']=r.id
  if obj['kind']=='Deployment':
   spec=obj['spec']['template']['spec'];c=spec['containers'][0]
   c['image']=stage(r,c['image']);c['imagePullPolicy']='IfNotPresent'
   c['resources']={'requests':{'cpu':'25m','memory':'64Mi'},'limits':{'cpu':'250m','memory':'192Mi'}}
   c['args']=[x for x in c['args'] if not x.startswith('--cert-dir') and not x.startswith('--kubelet-preferred-address-types')]+['--tls-cert-file=/serving/tls.crt','--tls-private-key-file=/serving/tls.key','--kubelet-certificate-authority=/kubelet-ca/ca.crt','--kubelet-preferred-address-types=InternalIP,Hostname']
   c['volumeMounts']=[{'name':'serving','mountPath':'/serving','readOnly':True},{'name':'kubelet-ca','mountPath':'/kubelet-ca','readOnly':True}]
   spec['volumes']=[{'name':'serving','secret':{'secretName':'b10b-metrics-server-tls'}},{'name':'kubelet-ca','configMap':{'name':'b10b-kubelet-ca'}}]
  if obj['kind']=='APIService':
   obj['spec'].pop('insecureSkipTLSVerify',None);obj['spec']['caBundle']=base64.b64encode(ca.encode()).decode()
 r.apply(docs)
 r.kubectl('rollout','status','deployment/metrics-server','-n','kube-system','--timeout=150s',timeout=160)
 r.kubectl('wait','--for=condition=Available','apiservice/v1beta1.metrics.k8s.io','--timeout=90s',timeout=100)
 for _ in range(30):
  try:
   value=json.loads(r.kubectl('get','--raw','/apis/metrics.k8s.io/v1beta1/nodes'))
   if value.get('items'):break
  except RuntimeError:pass
  time.sleep(3)
 else:raise RuntimeError('No actual kubelet resource samples')
 r.save('resource-metrics-ready',{'metrics':value,'source_sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'kubelet_tls_verified':True,'aggregation_tls_verified':True})
 print('Real resource metrics ready with CA validation on both TLS legs',flush=True)
if __name__=='__main__':
 from run import Run
 install(Run())
