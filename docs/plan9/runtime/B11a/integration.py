"""Normal product enrollment and direct-route verification for B11a."""
import argparse
import json
from manage import Run,write_json
from common.hosts import HostEnrollment
from artifact_release import VERSION


class Batch(HostEnrollment):
    def __init__(self,run,api):
        specs=json.loads((run.out/'host-specs.json').read_text(encoding='utf-8'))
        super().__init__(run,api,specs,'b11a')

    @property
    def host(self):return self.map['hosts']['direct']['host_id']

    @property
    def collector(self):return self.map['hosts']['direct']['collector_id']

    def converge(self):
        return self.api.wait('/enterprise/hosts/'+self.host+'/collector',
            lambda row:row['status']=='converged' and row['desired_revision']==row['effective_revision'],
            timeout=180,label='Direct host Collector convergence')

    def reinstall_failed_activation(self):
        """A failed first enrollment has no identity for the repair operation."""
        base='/enterprise/hosts/'+self.host+'/collector'
        current=self.api.call('GET',base)
        assert current['status']=='degraded' and current['effective_revision']==0
        self.run.save('failed-first-activation-'+str(current['version']),current)
        body={'distribution_version_id':current['distribution_version_id'],'profile_ids':[],
              'route_kind':'direct_argus','transport':'direct','expected_version':current['version']}
        self.api.confirm(self.api.call('POST',base+'/actions/preview-uninstall',body,
            key='uninstall-failed-'+str(current['version']),expected=201))
        current=self.api.wait(base,lambda x:x['status']=='uninstalled',timeout=120)
        body['expected_version']=current['version']
        self.api.confirm(self.api.call('POST',base+'/actions/preview-install',body,
            key='fresh-install-'+str(current['version']),expected=201))
        self.converge()
        print('Fresh signed Collector installation and enrollment succeeded',flush=True)

    def enroll(self):
        self.onboard()
        host=self.api.call('GET','/enterprise/hosts/'+self.host)
        assert host['control_path']=='direct' and not host.get('bastion_scope_id')
        current=self.api.call('GET','/enterprise/hosts/'+self.host+'/collector',expected=[200,204,404])
        distribution=next(row for row in self.api.call('GET','/enterprise/telemetry/distributions') if row['version']==VERSION and row['name']=='argus-otelcol-linux-amd64')
        if current and not current.get('code') and current['distribution_version_id']!=distribution['id']:
            preview=self.api.call('POST','/enterprise/hosts/'+self.host+'/collector/actions/preview-repair',{
                'distribution_version_id':distribution['id'],'profile_ids':[],'route_kind':'direct_argus',
                'transport':'direct','expected_version':current['version']},key='repair-current-host-artifact',expected=201)
            self.api.confirm(preview)
        self.collectors(distribution_version=VERSION)
        collector=self.api.call('GET','/enterprise/hosts/'+self.host+'/collector')
        assert collector['route']['kind']=='direct_argus' and collector['route']['transport']=='direct',collector
        self.builtins()
        self.converge()
        self.run.save('direct-route-verification',{'host_id':self.host,'collector_id':self.collector,
            'control_path':host['control_path'],'route_kind':collector['route']['kind'],
            'route_transport':collector['route']['transport'],'real_ssh_connection_test':self.map['hosts']['direct']['connection_test_id']})
        print('Actual direct SSH installation and direct Collector telemetry configured',flush=True)


if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['enroll','reinstall_failed_activation']);args=p.parse_args()
    run=Run();api=run.api();api.login();getattr(Batch(run,api),args.action)()
