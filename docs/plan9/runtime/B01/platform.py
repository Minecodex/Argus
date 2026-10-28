"""B01 installation and monitoring through actual Argus product APIs."""
import argparse
import json
from pathlib import Path
import sys

from manage import Run, write_json, now

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'common'))
from api import Api


from hosts import HostEnrollment

class B01(HostEnrollment):
    def __init__(self, run, api):
        super().__init__(run, api, {
            'host-a': {'letter':'A','ssh_port':22221,'name':'B01-Linux-A'},
            'host-b': {'letter':'B','ssh_port':22222,'name':'B01-Linux-B'},
        }, 'b01')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['onboard', 'collectors', 'builtins', 'dashboards', 'alerts', 'exercise', 'lifecycle', 'retain', 'review-scenario'])
    parser.add_argument('--run', default='p9-b01-20261026-01')
    parser.add_argument('--session', required=True)
    parser.add_argument('--ca', required=True)
    parser.add_argument('--credentials-stdin', action='store_true')
    args = parser.parse_args()
    credentials = json.loads(sys.stdin.readline()) if args.credentials_stdin else None
    run = Run(args.run)
    api = Api(run, args.session, args.ca, credentials)
    api.call('GET', '/enterprise/auth/session')
    if args.action == 'review-scenario':
        from scenario import Scenario
        results = {}
        for name in ['exercise', 'lifecycle', 'retain']:
            print(name + ': started', flush=True)
            run.event('review_stage_started', stage=name)
            value = getattr(Scenario(run, api), name)()
            results[name] = {'finished_at': now(), 'result': value}
            write_json(run.out / 'scenario-checks.json', results)
            api.save()
            print(name + ': completed', flush=True)
        result = results
    elif args.action in ('onboard', 'collectors', 'builtins'):
        target = B01(run, api)
        result = getattr(target, args.action)()
    else:
        from scenario import Scenario
        target = Scenario(run, api)
        result = getattr(target, args.action)()
    api.save()
    print(json.dumps(result), flush=True)
