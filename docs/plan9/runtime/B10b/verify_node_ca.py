"""Normal builtin rollout and native public CA-file/TLS rejection proof."""
import copy,hashlib,json,time,uuid
from datetime import datetime,timedelta,timezone
import yaml
from cryptography import x509
from cryptography.x509.oid import NameOID
from cryptography.hazmat.primitives import hashes,serialization
from cryptography.hazmat.primitives.asymmetric import ec
from run import Run,execute
r=Run();a=r.api();base='/enterprise/kubernetes-clusters/'+r.cluster_id
rows=a.call('GET','/monitoring/builtins?target_type=kubernetes_cluster&target_id='+r.cluster_id)
row=next(x for x in rows if x['definition']['key']=='kubernetes_node_metrics');current=row['collection']
ca=execute(['docker','exec',r.id+'-control-plane','cat','/etc/kubernetes/pki/ca.crt'])
a.confirm(a.call('POST','/monitoring/builtins/preview',{'target':{'kind':'kubernetes_cluster','id':r.cluster_id},'key':'kubernetes_node_metrics','enabled':True,'expected_version':current['version'],'config':{'interval':'15s','component_tls':{'ca_pem':ca}}},key='native-file-ca-rollout-'+str(current['version']),expected=201))
def converged(value):
 if value['status']=='degraded':raise RuntimeError('Native CA file candidate failed')
 return value['status']=='converged' and value['desired_revision']==value['effective_revision']
state=a.wait(base+'/collector',converged,timeout=240,label='Native CA-file rollout')
fresh=a.wait('/monitoring/collections/'+current['id'],lambda x:x['apply_status']=='effective' and x['health']=='healthy' and x['freshness']=='fresh',timeout=120,label='Explicit file CA actual sample')
agent=r.get('daemonset','argus-otelcol-agent','-n','argus-telemetry')
volume=next(v for v in agent['spec']['template']['spec']['volumes'] if v['name']=='config')
cm=r.get('configmap',volume['configMap']['name'],'-n','argus-telemetry')
assert cm['data']['kubelet-ca.pem'].strip()==ca.strip()
config=json.loads(cm['data']['agent.yaml']);receiver=next(v for k,v in config['receivers'].items() if k.startswith('kubeletstats/'))
assert receiver['ca_file']=='/etc/argus-otelcol/kubelet-ca.pem' and not receiver.get('insecure_skip_verify') and 'ca_pem' not in receiver
# A separate native-only probe uses the same scoped SA and wrong public CA.
# It has no Argus identity/exporter, never enters platform ingestion, and is removed.
key=ec.generate_private_key(ec.SECP256R1());name=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'B10b wrong CA probe')])
cert=x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(datetime.now(timezone.utc)-timedelta(minutes=1)).not_valid_after(datetime.now(timezone.utc)+timedelta(hours=1)).add_extension(x509.BasicConstraints(ca=True,path_length=None),critical=True).sign(key,hashes.SHA256())
wrong=cert.public_bytes(serialization.Encoding.PEM).decode();probe='b10b-wrong-ca-'+uuid.uuid4().hex[:6];ns='argus-telemetry';labels={'argus.plan9/run':r.id,'app':probe}
native={'receivers':{'kubeletstats/probe':{**receiver,'collection_interval':'5s','ca_file':'/probe/wrong-ca.pem'}},'exporters':{'nop':{}},'service':{'telemetry':{'logs':{'level':'info'}},'pipelines':{'metrics':{'receivers':['kubeletstats/probe'],'exporters':['nop']}}}}
serviceaccount=agent['spec']['template']['spec']['serviceAccountName']
image=agent['spec']['template']['spec']['containers'][0]['image']
r.apply([{'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':probe,'namespace':ns,'labels':labels},'data':{'config.json':json.dumps(native),'wrong-ca.pem':wrong}},
{'apiVersion':'batch/v1','kind':'Job','metadata':{'name':probe,'namespace':ns,'labels':labels},'spec':{'backoffLimit':0,'activeDeadlineSeconds':30,'template':{'metadata':{'labels':labels},'spec':{'serviceAccountName':serviceaccount,'restartPolicy':'Never','containers':[{'name':'probe','image':image,'imagePullPolicy':'Never','args':['--config=/probe/config.json'],'env':[{'name':'K8S_NODE_NAME','valueFrom':{'fieldRef':{'fieldPath':'spec.nodeName'}}}],'volumeMounts':[{'name':'probe','mountPath':'/probe','readOnly':True}],'resources':{'requests':{'cpu':'10m','memory':'32Mi'},'limits':{'cpu':'150m','memory':'96Mi'}}}],'volumes':[{'name':'probe','configMap':{'name':probe}}]}}}}])
try:
 deadline=time.monotonic()+35;logs=''
 while time.monotonic()<deadline:
  pods=r.get('pods','-n',ns,'-l','app='+probe)['items']
  if pods and pods[0].get('status',{}).get('phase') in ('Running','Failed'):
   logs=r.kubectl('logs','-n',ns,pods[0]['metadata']['name'])
   if 'certificate signed by unknown authority' in logs:break
  time.sleep(2)
 assert 'certificate signed by unknown authority' in logs,logs[-1200:]
 (r.out/'native-wrong-ca-probe.log').write_text(logs,encoding='utf-8')
 r.save('native-node-ca-verified',{'collector_revision':state['effective_revision'],'public_ca_sha256':hashlib.sha256(cm['data']['kubelet-ca.pem'].encode()).hexdigest(),'mounted_ca_file':receiver['ca_file'],'source_fresh':fresh['last_sample_at'],'wrong_ca_rejected':True,'tls_verification_enabled':True,'node_endpoint_unchanged':receiver['endpoint'],'probe_has_no_argus_identity_or_ingestion':True})
finally:
 for kind in ('job','configmap'):
  obj=r.get(kind,probe,'-n',ns);assert obj['metadata']['labels']['argus.plan9/run']==r.id
  r.kubectl('delete',kind,probe,'-n',ns,'--wait=true','--timeout=60s',timeout=70)
r.save('native-ca-probe-cleanup',{'owned_probe_deleted':True,'job':probe})
print('Explicit mounted public CA used; wrong CA rejected by actual native receiver; probe cleaned',flush=True)
