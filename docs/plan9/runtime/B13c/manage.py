"""Fresh B13c ownership and versioned evidence, reusing the native cache scene."""
import argparse
from pathlib import Path
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
sys.path.insert(0, str(HERE.parent))
from B13a.manage import Run as CacheRun, execute, write_json, now


class Run(CacheRun):
    RUN_ID = 'p9-b13c-20261118-01'
    BATCH = 'B13c'
    PACKAGE = HERE
    BUILD_CONTEXT = HERE.parent
    HOST_TAG = 'argus-plan9-lifecycle:b13c-20261118'
    PREDECESSOR = 'p9-b13b-20261118-01'
    AUTHORIZATION = '继续下一轮 B13c，把撤权、凭据轮换和资源生命周期按计划验收一遍。'


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['prepare', 'build', 'up', 'workload'])
    getattr(Run(), parser.parse_args().action)()
