"""Count complete native records, including compactions and failed scripts."""
import collections
import json
import subprocess
import uuid

from session_rerun import Rerun, OUT


def tool_metrics(events):
    calls = {}
    signatures = collections.Counter()
    counts = collections.Counter()
    results = []
    completed_jobs = set()
    completed_get_repeats = 0
    for event in events:
        payload = event['payload']
        if event['event_type'] == 'assistant_message':
            for call in payload.get('tool_calls', []):
                calls[call['id']] = call
        if event['event_type'] != 'tool_call_result':
            continue
        call = calls.get(payload.get('tool_call_id'), {})
        try:
            arguments = json.loads(call.get('arguments', '{}'))
        except (TypeError, ValueError):
            arguments = {}
        target = payload.get('target_tool_id') or payload.get('tool_id')
        signature = json.dumps([target, arguments], sort_keys=True, ensure_ascii=False)
        signatures[signature] += 1
        counts[target] += 1
        summary = payload.get('projection', {}).get('summary', {})
        protocol_failed = payload.get('status') != 'succeeded'
        exit_code = summary.get('exit_code') if isinstance(summary, dict) else None
        nonzero_script = target == 'bash' and exit_code not in [None, 0]
        domain_failed = isinstance(summary, dict) and summary.get('status') in ['failed', 'cancelled', 'result_unknown', 'timed_out']
        if target == 'telemetry.dashboard.query.get' and isinstance(summary, dict):
            job = summary.get('id')
            if job in completed_jobs:
                completed_get_repeats += 1
            if summary.get('status') == 'complete':
                completed_jobs.add(job)
        results.append({'sequence': event['sequence'], 'tool': target, 'status': payload.get('status'),
                        'error_code': payload.get('error_code') or (summary.get('error_code') if isinstance(summary, dict) else None),
                        'protocol_failed': protocol_failed, 'nonzero_script': nonzero_script,
                        'domain_failed': domain_failed, 'strict_failed': protocol_failed or nonzero_script or domain_failed,
                        'exit_code': exit_code})
    total = len(results)
    failed = sum(r['protocol_failed'] for r in results)
    strict_failed = sum(r['strict_failed'] for r in results)
    return {'total': total, 'succeeded': total - failed, 'failed': failed,
            'success_rate_pct': 100 * (total - failed) / total if total else None,
            'failure_rate_pct': 100 * failed / total if total else None,
            'strict_succeeded': total - strict_failed, 'strict_failed': strict_failed,
            'strict_success_rate_pct': 100 * (total - strict_failed) / total if total else None,
            'nonzero_script_count': sum(r['nonzero_script'] for r in results),
            'same_arguments_repeats_including_legal_polling': sum(n - 1 for n in signatures.values()),
            'gets_after_job_already_complete': completed_get_repeats,
            'by_tool': dict(counts), 'results': results}


def model_usage(run_ids):
    values = ','.join("'" + str(uuid.UUID(x)) + "'" for x in run_ids)
    sql = ("SELECT COALESCE(json_agg(x),'[]') FROM (SELECT run_id,model_id,model_revision,call_kind,"
           "count(*) AS calls,count(*) FILTER (WHERE status='failed') AS failed_calls,"
           "sum(input_tokens) AS input_tokens,sum(output_tokens) AS output_tokens,"
           "sum(cached_input_tokens) AS cached_input_tokens,sum(amount) AS amount_usd,"
           "bool_and(input_usage_source='provider' AND output_usage_source='provider') AS provider_input_output "
           "FROM model_calls WHERE run_id IN (" + values + ") GROUP BY run_id,model_id,model_revision,call_kind ORDER BY run_id,call_kind) x")
    output = subprocess.check_output(['kubectl', '--context', 'docker-desktop', 'exec', '-n', 'argus-system',
                                      'argus-postgresql-0', '--', 'psql', '-U', 'argus', '-d', 'argus', '-Atq', '-c', sql], text=True)
    return json.loads(output)


def totals(usage):
    result = {key: sum((row[key] or 0) for row in usage) for key in ['calls', 'failed_calls', 'input_tokens', 'output_tokens', 'cached_input_tokens', 'amount_usd']}
    result['total_tokens'] = result['input_tokens'] + result['output_tokens']
    result['compaction_calls'] = sum(row['calls'] for row in usage if row['call_kind'] == 'compaction')
    result['provider_input_output'] = all(row['provider_input_output'] for row in usage)
    return result


def measure(v):
    historical = json.loads((OUT / 'historical-baselines.json').read_text())
    events = v.events()
    v.save('ledger-measured', {'items': events})
    original_ids = [b['run_id'] for b in historical.values()]
    current_ids = [v.state[key + '_run'] for key in ['query', 'analysis'] if v.state.get(key + '_run')]
    usage = model_usage(original_ids + current_ids)
    v.save('model-call-usage-measured', usage)
    result = {'conversation_id': v.state['conversation_id'], 'scope': 'two B12 read-only tasks; no fault injection or remote mutation',
              'definitions': {'protocol_rate': 'outer native tool result status',
                              'strict_rate': 'also count nonzero Bash exit and failed terminal domain job',
                              'tokens': 'all model_calls for selected Runs, including compactions, failed and cancelled attempts; cache is not added twice',
                              'comparison_limits': 'Single observational rerun. Query prompt identical; recent time/data and inherited history differ. CSV/report is performed after querying using that evidence, whereas historical order was CSV then query.'},
              'cases': {}}
    for name, before in historical.items():
        current_id = v.state.get(name + '_run')
        if not current_id:
            continue
        current = [e for e in events if e.get('run_id') == current_id]
        old_usage = totals([r for r in usage if r['run_id'] == before['run_id']])
        new_usage = totals([r for r in usage if r['run_id'] == current_id])
        run = v.api.call('GET', '/runs/' + current_id)
        result['cases'][name] = {'baseline_run_id': before['run_id'], 'current_run_id': current_id,
                                 'status': run['status'], 'error_code': run.get('error_code'), 'stop_reason': run.get('stop_reason'),
                                 'before': {'tools': tool_metrics(before['events']), 'model': old_usage},
                                 'after': {'tools': tool_metrics(current), 'model': new_usage},
                                 'token_reduction_pct': 100 * (old_usage['total_tokens'] - new_usage['total_tokens']) / old_usage['total_tokens']}
    selected = list(result['cases'])
    old_events = [e for name in selected for e in historical[name]['events']]
    new_events = [e for e in events if e.get('run_id') in current_ids]
    base_usage = totals([r for r in usage if r['run_id'] in [historical[n]['run_id'] for n in selected]])
    new_usage = totals([r for r in usage if r['run_id'] in current_ids])
    result['overall'] = {'before': {'tools': tool_metrics(old_events), 'model': base_usage},
                         'after': {'tools': tool_metrics(new_events), 'model': new_usage},
                         'token_reduction_pct': 100 * (base_usage['total_tokens'] - new_usage['total_tokens']) / base_usage['total_tokens'],
                         'all_runs_succeeded': all(c['status'] == 'succeeded' for c in result['cases'].values()) and len(result['cases']) == 2}
    v.save('comparison', result)
    print(json.dumps({name: {'status': case['status'], 'tools': {k: case['after']['tools'][k] for k in ['total', 'failed', 'strict_failed']},
                             'model': case['after']['model'], 'token_reduction_pct': case['token_reduction_pct']}
                      for name, case in result['cases'].items()}, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    measure(Rerun())
