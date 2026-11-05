"""Normal application, native Redis collection, SDK admission and binding APIs."""
import argparse
from datetime import datetime,timedelta,timezone
import json
from pathlib import Path
import sys
import time
from urllib.parse import urlparse
from manage import Run,RUNS,execute,write_json,now
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from hosts import HostEnrollment
from monitoring import MonitoringBatch

class ApplicationBatch(MonitoringBatch):
    def __init__(self,run,api):super().__init__(run,api,['redis'])
    def commit(self,body,key):
        result=self.api.confirm(self.api.call('POST','/monitoring/commands/preview',body,key=key,expected=201))
        return result['resource_ref']['resource_id']
    def converge(self):
        path='/enterprise/hosts/'+self.map['hosts']['collector']['host_id']+'/collector'
        return self.api.wait(path,lambda c:c['status']=='converged' and c['desired_revision']==c['effective_revision'],timeout=180,label=f'{self.run.batch} Collector converged')
    def targets(self):
        self.converge();name=f'{self.run.batch}-Redis';collector=self.map['hosts']['collector']['collector_id'];creds=json.loads((self.run.out/'private/redis-credentials.json').read_text(encoding='utf-8'))
        rows=[c for c in self.api.call('GET','/monitoring/collections?search='+name)['items'] if c['name']==name];assert len(rows)<=1
        if not rows:
            self.commit({'kind':'collection','operation':'create','credentials':{'username':'monitor','password':creds['monitor']},'collection':{'name':name,'integration':'redis','collector_id':collector,'target_name':name,'target_identity':{'endpoint':'redis:6379'},'enabled':True,'credential_update':'replace','config':{'interval':'10s','timeout':'5s','tls':{'enabled':False}}}},'redis-collection')
            rows=[c for c in self.api.call('GET','/monitoring/collections?search='+name)['items'] if c['name']==name]
        assert len(rows)==1;c=rows[0];self.map['targets']['redis']={'name':name,'engine':'redis','host':'collector','target_id':c['target']['id'],'collection_id':c['id'],'collector_id':collector};self.save();self.converge()
        self.api.wait('/monitoring/collections/'+c['id'],lambda c:c['health']=='healthy' and c['freshness']=='fresh',timeout=180,label='Redis native data');print('Redis native metrics fresh',flush=True)
    def sdk_endpoint(self, app, state, suffix, name, protocol, signals, transport):
        self.converge()
        if 'endpoint_id' not in state:
            path = '/applications/' + app['id'] + '/ingest-endpoints'
            matches = [row for row in self.api.call('GET', path)['items'] if row['name'] == name]
            assert len(matches) <= 1
            endpoint = {
                'name': name, 'application_id': app['id'],
                'collector_id': self.map['hosts']['collector']['collector_id'],
                'protocol': protocol, 'signals': signals, 'enabled': True,
            }
            state['endpoint_id'] = matches[0]['id'] if matches else self.commit(
                {'kind': 'endpoint', 'operation': 'create', 'endpoint': endpoint}, 'orders-endpoint' + suffix)
            self.save()
        self.converge()
        path = '/monitoring/ingest-endpoints/' + state['endpoint_id'] + '/connection'
        connection = self.api.wait(path, lambda c: c['configuration_status'] == 'effective' and c['desired_enabled'],
                                   timeout=180, label='SDK TLS receiver ready')
        assert connection['tls']['required'] and connection['application_id'] == app['id']
        assert connection['protocol'] == protocol and set(connection['signals']) == set(signals)
        write_json(self.run.out / ('sdk-connection' + suffix + '.json'), connection)
        listener = next(item for item in connection['listeners'] if item['transport'] == transport)
        address = urlparse(listener['local_endpoint'])
        assert address.scheme == ('grpcs' if transport == 'grpc' else 'https')
        assert address.hostname == '127.0.0.1' and address.port == listener['port']
        if 'key_id' not in state:
            expiry = (datetime.now(timezone.utc) + timedelta(hours=24)).isoformat()
            state['key_id'] = self.commit({'kind': 'ingest_key', 'operation': 'create', 'ingest_key': {
                'endpoint_id': state['endpoint_id'], 'expires_at': expiry}}, 'orders-ingest-key' + suffix)
            state['key_expires_at'] = expiry
            self.save()
        return connection, address.netloc

    def application(self):
        name = f'{self.run.batch} Orders {self.run.protocol}'
        self.map.setdefault('applications', {})
        if 'orders' not in self.map['applications']:
            rows = [x for x in self.api.call('GET', '/applications?search=' + self.run.batch)['items'] if x['name'] == name]
            assert len(rows) <= 1
            application = {'name': name, 'environment': 'testing', 'owner_name': 'Plan9',
                           'description': f'Actual gateway and worker requests with Redis and {self.run.protocol} telemetry'}
            ident = rows[0]['id'] if rows else self.commit(
                {'kind': 'application', 'operation': 'create', 'application': application}, 'orders-application')
            self.map['applications']['orders'] = {'id': ident, 'name': name, 'environment': 'testing'}
            self.save()
        app = self.map['applications']['orders']
        jaeger = self.run.transport == 'jaeger'
        endpoint_name = f'{self.run.batch} native {self.run.protocol}' if jaeger else f'{self.run.batch} native OTLP {self.run.protocol}'
        connection, address = self.sdk_endpoint(app, app, '', endpoint_name, self.run.trace_source,
            ['traces'] if jaeger else ['metrics', 'traces', 'logs'], self.run.listener_transport)
        secret = self.run.out / 'private/sdk.json'
        config = json.loads(secret.read_text(encoding='utf-8')) if secret.exists() else {}
        config.update(Endpoint=address, CA=connection['tls']['ca_bundle_pem'], ServerName=connection['tls']['server_name'],
            RedisPassword=json.loads((self.run.out/'private/redis-credentials.json').read_text(encoding='utf-8'))['app'],
            Run=self.run.id, Transport=self.run.transport)
        endpoints = [('Key', app)]
        if jaeger:
            aux = app.setdefault('aux_ingest', {})
            aux_connection, aux_address = self.sdk_endpoint(app, aux, '-aux', f'{self.run.batch} OTLP metrics and logs',
                                                               'otlp', ['metrics', 'logs'], 'grpc')
            assert connection['tls']['ca_bundle_pem'] == aux_connection['tls']['ca_bundle_pem']
            assert connection['tls']['server_name'] == aux_connection['tls']['server_name']
            config['AuxEndpoint'] = aux_address
            endpoints.append(('AuxKey', aux))
        for field, endpoint in endpoints:
            if field not in config:
                config[field] = self.api.call('POST', '/monitoring/ingest-keys/' + endpoint['key_id'] + '/reveal',
                                              key='reveal-sdk-key' if field == 'Key' else 'reveal-sdk-aux-key')['key']
                write_json(secret, config)
        write_json(secret, config)
        self.run.shell('collector', 'install -d -m 700 /var/lib/plan9/private\n')
        execute(['docker','exec','-i',self.run.container('collector')['Id'],'python3','-c',"import sys,pathlib;p=pathlib.Path('/var/lib/plan9/private/sdk.json');p.write_bytes(sys.stdin.buffer.read());p.chmod(0o600)"],data=secret.read_text(encoding='utf-8'))
        if 'redis_binding_id' not in app:
            app['redis_binding_id']=self.commit({'kind':'binding','operation':'create','binding':{'application_id':app['id'],'target':{'kind':'monitoring_target','id':self.map['targets']['redis']['target_id']}}},'orders-redis-binding');self.save()
        print('Application, scoped SDK keys, TLS listeners and Redis association ready',flush=True)

    def services(self):
        app=self.map['applications']['orders'];rows=self.api.wait('/applications/services?application_ids='+app['id'],lambda r:len(r['items'])==2 and all(x.get('last_seen_at') for x in r['items']),timeout=180,label='Two real SDK services discovered')['items'];assert {x['service_name'] for x in rows}=={'orders-gateway','orders-worker'};app['services']=rows;self.save();write_json(self.run.out/'discovered-services.json',rows)
        guide=self.api.wait('/monitoring/ingest-endpoints/'+app['endpoint_id']+'/connection',lambda c:bool(c.get('last_received_at')),timeout=120,label='SDK received');write_json(self.run.out/'sdk-received.json',guide);print('Two services discovered from actual SDK observations',flush=True)
        if 'aux_ingest' in app:
            guide = self.api.wait('/monitoring/ingest-endpoints/' + app['aux_ingest']['endpoint_id'] + '/connection',
                lambda c: bool(c.get('last_received_at')), timeout=120, label='OTLP metrics and logs received')
            write_json(self.run.out/'sdk-received-aux.json', guide)
        if self.run.listener_transport == 'grpc':
            self.grpc_receipts()

    def grpc_receipts(self):
        expected = {
            '/jaeger.api_v2.CollectorService/PostSpans' if self.run.transport == 'jaeger' else
            '/opentelemetry.proto.collector.trace.v1.TraceService/Export',
            '/opentelemetry.proto.collector.metrics.v1.MetricsService/Export',
            '/opentelemetry.proto.collector.logs.v1.LogsService/Export',
        }
        receipts = {}
        deadline = time.monotonic() + 60
        for service in ('orders-gateway', 'orders-worker'):
            unit = f'plan9-app@{service}.service'
            invocation = self.run.shell('collector', f'systemctl show -p InvocationID --value {unit}\n').strip()
            assert len(invocation) == 32 and all(c in '0123456789abcdef' for c in invocation)
            while True:
                lines = self.run.shell('collector',
                    f"journalctl -u {unit} _SYSTEMD_INVOCATION_ID={invocation} --no-pager -o cat -g '^(jaeger|otlp)_grpc_ack ' -n 24 || true\n")
                observed = {}
                for line in lines.splitlines():
                    if line.startswith(('otlp_grpc_ack ', 'jaeger_grpc_ack ')):
                        record = json.loads(line.split(' ', 1)[1])
                        observed[record['rpc']] = record
                if expected <= observed.keys():
                    break
                if time.monotonic() >= deadline:
                    raise RuntimeError(f'{service} is missing gRPC Export acknowledgements: {expected - observed.keys()}')
                time.sleep(2)
            for rpc in expected:
                record = observed[rpc]
                assert record['tls_version'] == 0x0304 and record['alpn'] == 'h2'
                assert record['verified_chains'] > 0
                if rpc.endswith('/PostSpans'):
                    assert record['spans'] > 0
                else:
                    assert record['rejected'] == 0
            receipts[service] = observed
        write_json(self.run.out/'grpc-export-receipts.json', receipts)
        print('Both services received native gRPC acknowledgements for metrics, logs and traces over verified TLS 1.3', flush=True)

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['onboard','collectors','builtins','targets','application','services','dashboards','verify','alerts','exercise','lifecycle']);p.add_argument('--session',required=True);p.add_argument('--ca',required=True);p.add_argument('--batch',choices=RUNS,default='B07a');args=p.parse_args();run=Run(batch=args.batch);api=Api(run,args.session,args.ca);api.call('GET','/enterprise/auth/session')
    if args.action in ['onboard','collectors']:getattr(HostEnrollment(run,api,run.hosts,run.batch.lower()),args.action)()
    elif args.action=='builtins':
        for key in ['host_metrics','host_process_metrics','host_systemd_metrics','collector_metrics']:
            ApplicationBatch(run,api).converge();HostEnrollment(run,api,run.hosts,run.batch.lower()).builtins(selected=[key]);ApplicationBatch(run,api).converge()
    elif args.action in ['targets','application','services']:getattr(ApplicationBatch(run,api),args.action)()
    elif args.action=='dashboards':
        from dashboard import dashboards
        dashboards(ApplicationBatch(run,api))
    else:
        from scenario import Scenario
        getattr(Scenario(run,api),args.action)()
    api.save()
