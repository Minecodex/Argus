"""Independent, read-only oracle for B12 immutable dashboard originals and CSV."""
import csv
from datetime import datetime, timezone
import hashlib
import io
import json
import math
from pathlib import Path
import re

from closeout import Closeout


def download(v, path, expected_hash, expected_bytes):
    response = v.api.http.get(v.api.origin + '/api/v1' + path, timeout=45)
    response.raise_for_status()
    body = response.content
    assert len(body) == expected_bytes and hashlib.sha256(body).hexdigest() == expected_hash
    return body


def utc_difference(actual, expected):
    reported = datetime.fromisoformat(actual.replace('Z', '+00:00'))
    original = datetime.fromisoformat(expected.replace('Z', '+00:00'))
    assert reported.tzinfo is not None and original.tzinfo is not None
    return abs((reported-original).total_seconds())


def observation_window(points):
    if not points:
        return None, None
    values = [float(point[0]) for point in points]
    render = lambda value: datetime.fromtimestamp(value, timezone.utc).isoformat()
    return render(min(values)), render(max(values))


def sources(v):
    events = v.events()
    chat = v.state['conversation_id']
    folder = v.private / 'sources'
    folder.mkdir(exist_ok=True)
    listed = v.api.call('GET', '/conversations/' + chat + '/workspace/files')
    listed = listed['items'] if isinstance(listed, dict) else listed
    registered = {file['id']: file for file in listed}
    jobs = {}
    for event in events:
        payload = event['payload']
        if event['event_type'] == 'tool_call_result' and payload.get('target_tool_id') == 'telemetry.dashboard.query.get' and payload.get('status') == 'succeeded':
            summary = payload.get('projection', {}).get('summary', {})
            if isinstance(summary, dict) and summary.get('status') == 'complete':
                jobs[summary['id']] = summary
    assert len(jobs) == 2, 'Exactly two scoped query jobs must complete'
    panels = []
    manifests = []
    citations = []
    for job in jobs.values():
        originals = {}
        for file in job['files']:
            body = download(v, '/conversations/' + chat + '/workspace/files/' + file['workspace_file_id'] + '/content', file['sha256'], file['bytes'])
            (folder / (hashlib.sha256(file['id'].encode()).hexdigest()[:20] + '.json')).write_bytes(body)
            originals[file['id']] = json.loads(body)
            native_file = registered[file['workspace_file_id']]
            assert native_file['content_hash'] == file['sha256']
            citations.append({'path': native_file['path'], 'sha256': file['sha256'], 'kind': file['kind']})
        manifest = originals[next(f['id'] for f in job['files'] if f['kind'] == 'manifest')]
        assert manifest['complete'] and not manifest['execution'].get('partial')
        execution = manifest['execution']
        resource = execution['resources'][0]
        assert len(execution['resources']) == 1
        assert resource['id'] in ['01a1e1dd-d568-724d-8d0b-40098d812b43', '3b24204d-9d9d-5c2a-b3fa-e37d621183a8']
        definition = manifest['definition']
        definitions = {p['id']: p for p in definition.get('panels', definition.get('spec', {}).get('panels', []))}
        statuses = {p['id']: p for p in execution['panels']}
        assert all(p['status'] in ['success', 'no_data'] for p in statuses.values())
        manifests.append({'job_id': job['id'], 'manifest_keys': list(manifest), 'definition_keys': list(definition), 'definition_panels': [{'id': p['id'], 'title': p.get('title'), 'unit': p.get('unit')} for p in definitions.values()]})
        for file in [f for f in job['files'] if f['kind'] == 'data']:
            raw = originals[file['id']]
            assert isinstance(raw, list)
            rows = [point for series in raw for point in series.get('values', [series['value']] if 'value' in series else [])]
            numeric = [float(point[1]) for point in rows]
            assert all(math.isfinite(n) for n in numeric)
            panel = definitions[file['panel_id']]
            first_point, last_point = observation_window(rows)
            record = {'job_id': job['id'], 'resource_id': resource['id'], 'resource_name': resource.get('name'),
                      'panel_id': file['panel_id'], 'target_id': file.get('target_id'), 'title': panel.get('title'), 'unit': panel.get('unit'),
                      'status': statuses[file['panel_id']]['status'], 'from': execution['from'], 'to': execution['to'],
                      'first_point_at': first_point, 'last_point_at': last_point,
                      'point_count': len(numeric), 'value_min': min(numeric) if numeric else None, 'value_max': max(numeric) if numeric else None,
                      'last_value': float(max(rows, key=lambda point: float(point[0]))[1]) if rows else None, 'sha256': file['sha256']}
            if record['status'] == 'no_data':
                assert not numeric
            panels.append(record)
    assert len(panels) == 6
    file = v.state['csv_file']
    body = download(v, '/conversations/' + chat + '/workspace/files/' + file['id'] + '/content', file['content_hash'], file['byte_size'])
    rows = list(csv.DictReader(io.StringIO(body.decode('utf-8-sig'))))
    expected = {'command_delta': int(rows[-1]['total_commands_processed']) - int(rows[0]['total_commands_processed']),
                'peak_memory_bytes': max(int(row['used_memory_bytes']) for row in rows),
                'csv_from': rows[0]['observed_at'], 'csv_to': rows[-1]['observed_at']}
    proof = {'source_hashes_verified': True, 'panels': panels, 'csv': expected, 'manifests': manifests,
             'citations': citations + [{'path': file['path'], 'sha256': file['content_hash'], 'kind': 'csv'}],
             'verification_only': True, 'model_report_modified': False}
    v.save('independent-source-oracle', proof)
    print(json.dumps({'panels': panels, 'csv': expected}), flush=True)
    return proof


def verify(v):
    proof = sources(v)
    chat = v.state['conversation_id']
    report_run = v.state.get('references_run') or v.state.get('analysis_run')
    deliveries = [e['payload']['artifact'] for e in v.events() if e['event_type'] == 'artifact_published' and e.get('run_id') == report_run]
    assert deliveries, 'The model must publish a real delivery'
    artifact = deliveries[-1]
    body = download(v, '/conversations/' + chat + '/deliveries/' + artifact['id'] + '/content', artifact['content_hash'], artifact['byte_size'])
    (v.private / 'published-closeout.md').write_bytes(body)
    text = body.decode('utf-8')
    values = [json.loads(block) for block in re.findall(r'```json\s*(.*?)```', text, re.S)]
    result = next(value for value in values if isinstance(value, dict) and 'command_delta' in value and 'panels' in value)
    assert result['command_delta'] == proof['csv']['command_delta']
    assert result['peak_memory_bytes'] == proof['csv']['peak_memory_bytes']
    seen = set()
    time_precision = []
    for expected in proof['panels']:
        key = expected['resource_id'], expected['panel_id']
        actual = next(p for p in result['panels'] if (p.get('resource_id'), p.get('panel_id')) == key)
        assert key not in seen
        seen.add(key)
        for field in ['unit', 'point_count']:
            assert actual[field] == expected[field], (key, field, actual[field], expected[field])
        assert type(actual['point_count']) is int
        # A returned observation range is different from the requested query
        # window for instant panels and partially populated time series. Both
        # are recorded independently; a report must match one complete pair.
        ranges = [('query', expected['from'], expected['to'])]
        if expected['point_count']:
            ranges.insert(0, ('observations', expected['first_point_at'], expected['last_point_at']))
        matched = next((kind for kind, start, end in ranges if utc_difference(actual['from'], start) < .001 and utc_difference(actual['to'], end) < .001), None)
        assert matched, (key, actual['from'], actual['to'], ranges)
        for field in ['from', 'to']:
            expected_time = expected['first_point_at' if field == 'from' else 'last_point_at'] if matched == 'observations' else expected[field]
            difference = utc_difference(actual[field], expected_time)
            # The UI/report may display milliseconds while the sealed manifest
            # retains nanoseconds. Compare instants, not timestamp spelling.
            assert difference < .001, (key, field, actual[field], expected[field])
            time_precision.append({'resource_id': key[0], 'panel_id': key[1], 'field': field, 'reported': actual[field], 'original': expected_time, 'window_kind': matched, 'difference_seconds': difference})
        for field in ['value_min', 'value_max', 'last_value']:
            if expected[field] is None:
                assert actual[field] is None
            else:
                assert type(actual[field]) in [int, float]
                if float(expected[field]).is_integer():
                    assert actual[field] == expected[field], (key, field, actual[field], expected[field])
                else:
                    assert math.isclose(actual[field], expected[field], rel_tol=1e-6, abs_tol=1e-8), (key, field, actual[field], expected[field])
        if expected['status'] == 'no_data':
            assert actual['status'] == 'no_data'
    assert len(result['panels']) == len(seen) == 6
    if v.state.get('references_run'):
        for citation in proof['citations']:
            assert citation['path'] in text and citation['sha256'] in text, citation
    v.save('report-verification', {'passed': True, 'artifact': artifact, 'all_six_panel_values_units_windows_verified': True,
                                    'csv_exact_integers_verified': True, 'narrative_semantic_review': 'pending separate review of the exact bytes',
                                    'utc_bounds_verified_within_one_millisecond': time_precision,
                                    'report_modified_by_verifier': False})
    print('Model report data verified against immutable source originals; narrative review remains separate', flush=True)


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('phase', choices=['sources', 'report'])
    args = parser.parse_args()
    value = Closeout()
    {'sources': sources, 'report': verify}[args.phase](value)
