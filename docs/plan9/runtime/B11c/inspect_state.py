"""Small read-only diagnosis; never prints credentials or session cookies."""
from integration import Batch
from manage import Run
import json
run=Run();api=run.api();api.login();batch=Batch(run,api)
for key,state in batch.map['hosts'].items():
    host=api.call('GET','/enterprise/hosts/'+state['host_id'])
    rows=api.call('GET','/monitoring/builtins?target_type=host&target_id='+state['host_id'])
    print(key,host['connection_status'],host['resource_version'])
    print(json.dumps([{k:row.get('collection',{}).get(k) for k in ['integration','desired_enabled','health','freshness','last_sample_at','error_code','version']} for row in rows if row.get('collection',{}).get('desired_enabled')]))
    print(run.shell(key,'systemctl is-active argus-connector.service argus-otelcol.service\ncat /var/lib/argus-otelcol/identity/delivery-state.json\n'))
