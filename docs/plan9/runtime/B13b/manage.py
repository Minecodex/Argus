"""Fresh B13b scene using the shared two-host/cache ownership lifecycle."""
import argparse
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B13a.manage import Run as CacheRun, execute, write_json, now


class Run(CacheRun):
    RUN_ID = 'p9-b13b-20261118-01'
    BATCH = 'B13b'
    PACKAGE = HERE
    HOST_TAG = 'argus-plan9-alerts:b13b-20261118'
    PREDECESSOR = 'p9-b13a-20261117-01'
    AUTHORIZATION = '下一轮'

    def prepare(self):
        super().prepare()
        (self.private / 'receiver').mkdir(exist_ok=True)


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['prepare', 'build', 'up', 'workload'])
    getattr(Run(), p.parse_args().action)()
