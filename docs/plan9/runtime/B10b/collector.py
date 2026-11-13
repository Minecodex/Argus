"""Explicit public node CA; no filesystem assumption or TLS bypass."""
from infrastructure import execute

def builtins(r,b):
 ca=execute(['docker','exec',r.id+'-control-plane','cat','/etc/kubernetes/pki/ca.crt'])
 base='/monitoring/builtins?target_type=kubernetes_cluster&target_id='+b.cluster
 for key in ('kubernetes_cluster_metrics','kubernetes_node_metrics','collector_metrics'):
  b.converge();row=next(x for x in b.api.call('GET',base) if x['definition']['key']==key)
  current=row.get('collection') or {};config={'interval':'10s'}
  if key=='kubernetes_node_metrics':config['component_tls']={'ca_pem':ca}
  if not current.get('desired_enabled') or current.get('config')!=config:
   b.api.confirm(b.api.call('POST','/monitoring/builtins/preview',{'target':{'kind':'kubernetes_cluster','id':b.cluster},'key':key,'enabled':True,'expected_version':current.get('version',0),'config':config},key='enable-public-ca-'+key+'-'+str(current.get('version',0)),expected=201))
  b.converge();row=next(x for x in b.api.call('GET',base) if x['definition']['key']==key)
  b.map.setdefault('builtins',{})[key]={'id':row['collection']['id']};b.save()
 for key,state in b.map['builtins'].items():
  value=b.api.wait('/monitoring/collections/'+state['id'],lambda x:x['apply_status']=='effective' and x['freshness']=='fresh' and x['health']=='healthy',timeout=180,label=key+' real sample')
  r.save(key+'-fresh',value)
 print('All three native builtin metric sources fresh with verified kubelet TLS',flush=True)
