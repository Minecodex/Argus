"""Independent, read-only oracle for the real B12 model's delivered report."""
import csv
import hashlib
import io
import json
import math
import re

from context_runtime_verify import Verification


def verify(v):
    chat = v.state['conversation_id']
    folder = v.private / 'oracle-sources'
    folder.mkdir(exist_ok=True)

    def content(path, expected_hash, size):
        response = v.api.http.get(v.api.origin + '/api/v1' + path, timeout=45)
        response.raise_for_status()
        body = response.content
        assert len(body) == size
        assert hashlib.sha256(body).hexdigest() == expected_hash
        return body

    delivery = v.state['corrected_delivery']
    body = content('/conversations/' + chat + '/deliveries/' + delivery['id'] + '/content',
                   delivery['content_hash'], delivery['byte_size'])
    (v.private / 'corrected-published-report.md').write_bytes(body)
    blocks = [json.loads(s) for s in re.findall(r'```json\s*(.*?)```', body.decode(), re.S)]
    reported = next(b for b in blocks if isinstance(b, list))
    reported_csv = next(b for b in blocks if isinstance(b, dict) and 'command_delta' in b)
    assert len(reported) == 6
    checked = []
    for job in v.state['jobs']:
        raw_files = {}
        for file in job['files']:
            data = content('/conversations/' + chat + '/workspace/files/' + file['workspace_file_id'] + '/content',
                           file['sha256'], file['bytes'])
            (folder / (file['id'] + '.json')).write_bytes(data)
            raw_files[file['id']] = json.loads(data)
        manifest_file = next(f for f in job['files'] if f['kind'] == 'manifest')
        manifest = raw_files[manifest_file['id']]
        assert manifest['complete'] and manifest['job_id'] == job['id']
        resource = manifest['execution']['resources'][0]
        for file in [f for f in job['files'] if f['kind'] == 'data']:
            origin = next(f for f in manifest['files'] if f['id'] == file['id'])
            assert origin['target_id'] == file['target_id']
            series = raw_files[file['id']]
            assert len(series) == 1, 'This B12 oracle requires its actual single-resource series'
            row = series[0]
            points = row['values'] if 'values' in row else [row['value']]
            values = [float(p[1]) for p in points]
            assert all(math.isfinite(x) for x in values)
            result = next(r for r in reported if r['resource_id'] == resource['id'] and r['panel_id'] == file['panel_id'])
            assert result['resource_type'] == resource['type'] and result['target_id'] == origin['target_id']
            expected = {'point_count': len(values), 'value_min': min(values), 'value_max': max(values), 'last_value': values[-1]}
            for key, value in expected.items():
                assert result[key] == value, (file['panel_id'], key, result[key], value)
            checked.append({'job_id': job['id'], 'resource_id': resource['id'], 'panel_id': file['panel_id'],
                            'target_id': origin['target_id'], 'source_sha256': file['sha256'], **expected})
    file = v.state['csv_file']
    data = content('/conversations/' + chat + '/workspace/files/' + file['id'] + '/content', file['content_hash'], file['byte_size'])
    rows = list(csv.DictReader(io.StringIO(data.decode('utf-8-sig'))))
    delta = int(rows[-1]['total_commands_processed']) - int(rows[0]['total_commands_processed'])
    peak = max(int(r['used_memory_bytes']) for r in rows)
    assert reported_csv['command_delta'] == delta == 37
    assert reported_csv['peak_memory_bytes'] == peak == 1562696
    assert reported_csv['source_sha256'] == file['content_hash']
    proof = {'passed': True, 'model_report_modified_by_verifier': False, 'delivery': delivery,
             'panels': checked, 'csv': {'command_delta': delta, 'peak_memory_bytes': peak},
             'limitations': 'Historical query evaluation points do not prove raw ingestion coverage; CSV includes monitoring commands and has a different time window.'}
    v.save('independent-report-oracle', proof)
    print('Independent oracle passed: six panel statistics, exact target/resource identities, hashes, and CSV integers')


if __name__ == '__main__':
    verify(Verification())
