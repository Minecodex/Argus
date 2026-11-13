import json,subprocess,base64
from run import Run
import yaml
r=Run()
pods=r.get('pods','--all-namespaces')['items']
print('States',[(x['metadata']['namespace'],x['metadata']['name'],x['status']['phase'],[(c['name'],c.get('state')) for c in x['status'].get('containerStatuses',[])]) for x in pods if x['metadata']['namespace'] in ('argus-telemetry','b10b-infra')])
for x in pods:
 if x['metadata']['namespace']=='argus-telemetry' and 'gateway' in x['metadata']['name']:
  try:print(r.kubectl('logs','-n','argus-telemetry',x['metadata']['name'],'--tail=35')[-9000:])
  except RuntimeError as error:print('Native log diagnostic:',str(error)[-700:])
docs=[x for x in yaml.safe_load_all((r.private/'rawfile-manifest.yaml').read_text(encoding='utf-8')) if x and x['kind']!='Secret']
for x in docs:
 if x['kind'] not in ('ClusterRole','ClusterRoleBinding','CSIDriver','StorageClass','CustomResourceDefinition'):x['metadata']['namespace']='b10b-infra'
p=subprocess.run(['docker','exec','-i',r.id+'-control-plane','kubectl','--kubeconfig=/etc/kubernetes/admin.conf','apply','--dry-run=server','-f','-'],input=json.dumps({'apiVersion':'v1','kind':'List','items':docs}).encode(),capture_output=True,timeout=30)
message=p.stderr.decode(errors='replace')
value=yaml.safe_load((r.private/'rawfile-values.yaml').read_text(encoding='utf-8'))['auth']['token']
for token in (value,base64.b64encode(value.encode()).decode()):message=message.replace(token,'[REDACTED]')
print('Storage public dry-run status',p.returncode,message[-3500:])
