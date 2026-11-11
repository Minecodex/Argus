"""Retire the fresh B09b run through the shared typed product cleanup."""
import sys

from run import ROOT


def retire(run, api):
    assert run.infra.id==run.id, 'Retire a shared historical environment through its owning run'
    sys.path.insert(0,str(ROOT/'docs/plan9/runtime/B09a'))
    from B09a.integration import Batch
    from B09a.retire import retire as retire_infrastructure
    retire_infrastructure(Batch(run.infra,api))
