"""Independently verify live query files and the real model's CSV report."""
import csv
import hashlib
import io
import json
import math
import re

from session_rerun import Rerun


def verify(v):
    events = v.events()
    chat = v.state['conversation_id']
    folder = v.private / 'sources'
    folder.mkdir(exist_ok=True)

    def fetch(path, digest, size):
        response = v.api.http.get(v.api.origin + '/api/v1' + path, timeout=45)
        response.raise_for_status()
        data = response.content
        assert len(data) == size and hashlib.sha256(data).hexdigest() == digest
        return data

    jobs = {}
    for event in events:
        p = event['payload']
        if event['event_type'] == 'tool_call_result' and p.get('target_tool_id') == 'telemetry.dashboard.query.get' and p.get('status') == 'succeeded':
            summary = p.get('projection', {}).get('summary', {})
            if summary.get('status') == 'complete':
                jobs[summary['id']] = summary
    assert len(jobs) >= 2
    panels = []
    no_data_panels = []
    for job in jobs.values():
        files = {}
        for file in job['files']:
            data = fetch('/conversations/' + chat + '/workspace/files/' + file['workspace_file_id'] + '/content', file['sha256'], file['bytes'])
            (folder / (file['id'] + '.json')).write_bytes(data)
            files[file['id']] = json.loads(data)
        manifest = files[next(f['id'] for f in job['files'] if f['kind'] == 'manifest')]
        assert manifest['complete']
        statuses = {p['id']: p['status'] for p in manifest['execution']['panels']}
        assert all(status in ['success', 'no_data'] for status in statuses.values())
        resource = manifest['execution']['resources'][0]
        assert resource['id'] in ['01a1e1dd-d568-724d-8d0b-40098d812b43', '3b24204d-9d9d-5c2a-b3fa-e37d621183a8']
        for file in [f for f in job['files'] if f['kind'] == 'data']:
            source = files[file['id']]
            points = [p for series in source for p in series.get('values', [series['value']] if 'value' in series else [])]
            if statuses[file['panel_id']] == 'no_data':
                assert not points
                no_data_panels.append(file['panel_id'])
                panels.append({'resource_id': resource['id'], 'panel_id': file['panel_id'], 'status': 'no_data',
                               'point_count': 0, 'sha256': file['sha256'], 'from': manifest['execution']['from'], 'to': manifest['execution']['to']})
                continue
            assert points
            values = [float(p[1]) for p in points]
            assert all(math.isfinite(n) for n in values)
            panels.append({'resource_id': resource['id'], 'panel_id': file['panel_id'], 'sha256': file['sha256'],
                           'point_count': len(values), 'min': min(values), 'max': max(values), 'last': values[-1],
                           'from': manifest['execution']['from'], 'to': manifest['execution']['to']})
    coverage = {(p['resource_id'], p['panel_id']) for p in panels}
    host = '01a1e1dd-d568-724d-8d0b-40098d812b43'
    redis = '3b24204d-9d9d-5c2a-b3fa-e37d621183a8'
    assert all((host, name) in coverage for name in ['cpu', 'memory'])
    assert all((redis, name) in coverage for name in ['memory', 'commands', 'hit_ratio'])
    deliveries = [e['payload']['artifact'] for e in events if e['event_type'] == 'artifact_published' and e.get('run_id') == v.state['analysis_run']]
    assert deliveries
    report = deliveries[-1]
    body = fetch('/conversations/' + chat + '/deliveries/' + report['id'] + '/content', report['content_hash'], report['byte_size'])
    (v.private / 'published-report.md').write_bytes(body)
    text = body.decode()
    blocks = [json.loads(s) for s in re.findall(r'```json\s*(.*?)```', text, re.S)]
    computed = next(b for b in blocks if isinstance(b, dict) and 'command_delta' in b and 'peak_memory_bytes' in b)
    file = v.state['csv_file']
    csv_data = fetch('/conversations/' + chat + '/workspace/files/' + file['id'] + '/content', file['content_hash'], file['byte_size'])
    rows = list(csv.DictReader(io.StringIO(csv_data.decode('utf-8-sig'))))
    assert computed['command_delta'] == int(rows[-1]['total_commands_processed']) - int(rows[0]['total_commands_processed']) == 37
    assert computed['peak_memory_bytes'] == max(int(r['used_memory_bytes']) for r in rows) == 1562696
    assert 'B12-Cache-Host' in text and 'B12-Redis' in text
    if 'hit_ratio' in no_data_panels:
        assert '命中率' in text and ('无数据' in text or '空结果' in text)
    proof = {'source_and_csv_verification_passed': True, 'report_modified_by_verifier': False, 'panels': panels, 'delivery': report,
             'csv_rows': len(rows), 'command_delta': 37, 'peak_memory_bytes': 1562696,
             'no_data_panels': no_data_panels, 'full_report_semantic_review_passed': None,
             'review_status': 'pending independent review of this exact report; source/CSV checks do not certify all narrative claims',
             'limits': 'Nonempty evaluation points do not prove complete raw ingestion; CSV and live panels have different windows.'}
    v.save('evidence-verification', proof)
    v.state['delivery'] = report
    v.save_state()
    print('Source hashes, explicit no-data and CSV integers verified; full report semantics require separate review')


if __name__ == '__main__':
    verify(Rerun())
