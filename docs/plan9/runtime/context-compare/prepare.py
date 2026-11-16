"""Freeze the actual B12 compression boundary and download authorized originals."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
OUT = Path(os.environ.get('ARGUS_CONTEXT_COMPARE_OUT',
    'C:/Users/23574/.codex/artifacts/argus-b12-context-compare-20261116'))


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')


def prepare():
    sys.path.insert(0, str(HERE.parent/'B12'))
    from manage import Run
    OUT.mkdir(parents=True, exist_ok=True)
    private = OUT/'private'
    private.mkdir(exist_ok=True)
    identity = subprocess.check_output(['whoami'], text=True).strip()
    subprocess.run(['icacls', str(private), '/inheritance:r', '/grant:r',
                    identity+':(OI)(CI)F', '*S-1-5-18:(OI)(CI)F'], check=True, capture_output=True)
    run = Run()
    api = run.api()
    api.login()
    mapping = json.loads(run.resources.read_text(encoding='utf-8'))
    audit = Path('C:/Users/23574/.codex/artifacts/argus-b12-tool-audit-20261116')
    events = json.loads((audit/'private/ledger.json').read_text(encoding='utf-8'))['items']
    snapshot = next(x for x in json.loads((audit/'private/context-snapshots.json').read_text(encoding='utf-8'))
                    if x['id'] == '01a1e215-40a8-7b8d-9a9e-3fa3d45b659c')
    user = next(x for x in events if x['sequence'] == 86)
    results = [x for x in events if x['sequence'] in [150, 152]]
    assert snapshot['source_through_sequence'] == results[-1]['sequence']
    registered = api.call('GET', '/conversations/'+mapping['conversation']['id']+'/workspace/files')
    registered = registered['items'] if isinstance(registered, dict) else registered
    registered = {x['id']: x for x in registered}
    files = []
    for event in results:
        job = event['payload']['projection']['summary']
        assert job['status'] == 'complete'
        for ref in job['files']:
            ident = ref['workspace_file_id']
            actual = registered[ident]
            assert actual['path'] == ref['path'] and actual['content_hash'] == ref['sha256']
            response = api.http.get(api.origin+'/api/v1/conversations/'+mapping['conversation']['id']+
                                    '/workspace/files/'+ident+'/content', timeout=45)
            response.raise_for_status()
            body = response.content
            assert len(body) == ref['bytes'] and hashlib.sha256(body).hexdigest() == ref['sha256']
            (private/(ident+'.json')).write_bytes(body)
            files.append({**ref, 'file_id': ident, 'job_id': job['id'], 'source_sequence': event['sequence'],
                          'source_result_ref': event['payload']['result_ref'],
                          'media_type': actual['media_type']})
    model = api.call('GET', '/enterprise/ai-models/'+mapping['model']['id'])
    assert model['model_id'] == 'glm-5.3-flash' and model['capabilities']['reasoning_effort'] == 'low'
    value = {'source_conversation': mapping['conversation']['id'], 'authorization_scope': results[0]['payload']['authorization_scope'],
             'workspace_id': files[0]['workspace_id'], 'through_sequence': snapshot['source_through_sequence'],
             'user_message': user['payload']['content'], 'historical_summary': snapshot['narrative_summary'],
             'snapshot_id': snapshot['id'], 'files': files, 'model': {k:model[k] for k in [
                 'id', 'name', 'model_id', 'revision', 'base_url', 'context_window_tokens', 'max_output_tokens', 'capabilities']}}
    save(private/'dataset.json', value)
    save(OUT/'source-proof.json', {'source': 'Normal authenticated reads of existing native sealed B12 query files',
         'snapshot_id': snapshot['id'], 'input_hash': hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest(),
         'files': [{k:x[k] for k in ['file_id','path','sha256','bytes','kind','panel_id']} for x in files],
         'model_configuration': value['model'], 'provider_key_copied': False,
         'original_conversation_mutated': False, 'synthetic_metric_samples': False})
    print('Frozen actual compaction boundary and '+str(len(files))+' authorized originals; six data files, two manifests')


if __name__ == '__main__':
    prepare()
