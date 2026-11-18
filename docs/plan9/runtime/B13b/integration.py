"""Normal B13b enrollment; wait for each collection activation before continuing."""
import argparse
import json
from manage import Run
from B13a.integration import Batch as CacheBatch
from common.hosts import HostEnrollment


class Batch(CacheBatch):
    NAME = 'B13b'

    def builtins(self):
        self.run.save('delivery-before-builtins', self.delivery())
        result = HostEnrollment.builtins(self, selected=['collector_metrics', 'host_metrics', 'host_process_metrics', 'host_systemd_metrics'])
        self.run.save('delivery-after-builtins', self.delivery())
        return result

    def delivery(self):
        return {key: json.loads(self.run.shell(key, 'cat /var/lib/argus-otelcol/identity/delivery-state.json\n'))['report'] for key in self.map['hosts']}

    def dashboards(self):
        from B13a.dashboards import defaults
        defaults(self)


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['onboard', 'collectors', 'builtins', 'targets', 'applications', 'dashboards'])
    run = Run()
    api = run.api()
    api.login()
    batch = Batch(run, api)
    action = p.parse_args().action
    if action == 'collectors':
        batch.collectors('0.3.3-b11a-20261113')
    else:
        getattr(batch, action)()
