"""Normal lifecycle scene enrollment, using the previously verified native receivers."""
import argparse
from manage import Run
from B13b.integration import Batch as AlertBatch


class Batch(AlertBatch):
    NAME = 'B13c'


if __name__ == '__main__':
    from B13b.network import configure
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['network', 'onboard', 'collectors', 'builtins', 'targets', 'applications', 'dashboards'])
    action = parser.parse_args().action
    run = Run()
    if action == 'network':
        configure(run)
    else:
        api = run.api()
        api.login()
        batch = Batch(run, api)
        if action == 'collectors':
            batch.collectors('0.3.3-b11a-20261113')
        else:
            getattr(batch, action)()
