"""Offline comparison and independent facts; never dispatches a model/tool."""
import hashlib
import json
import re
from pathlib import Path

from prepare import OUT, save
from run import references


def file_failure_counts(messages):
    failed_calls, failed_opens = 0, 0
    for message in messages:
        if message['role']!='tool':
            continue
        result = json.loads(message['content'])['summary']
        if result.get('error_code')=='WORKSPACE_FILE_NOT_FOUND':
            failed_calls += 1
            failed_opens += 1
        elif result.get('exit_code',0)!=0:
            count = len(re.findall(r'^FileNotFoundError:',result.get('stderr',''),re.MULTILINE))
            if count:
                failed_calls += 1
                failed_opens += count
    return {'path_failure_tool_calls':failed_calls,'file_open_failures':failed_opens}


def main():
    comparison = json.loads((OUT/'comparison.json').read_text(encoding='utf-8'))
    assert comparison['complete']
    cases = comparison['cases']
    dataset = json.loads((OUT/'private/dataset.json').read_text(encoding='utf-8'))
    frozen_refs = json.loads((OUT/'candidate-reference-index.json').read_text(encoding='utf-8'))
    assert frozen_refs == references(dataset), 'Post-test source hardening changed the paid experiment input'
    original = None
    code_receipts = []
    for case in cases:
        name = case['case']
        initial = json.loads((OUT/'private'/name/'initial-messages.json').read_text(encoding='utf-8'))
        facts = json.loads(initial[3]['content'].split('Server execution facts (data): ',1)[1])
        if case['arm']=='candidate':
            assert facts.pop('context_references') == frozen_refs
        initial[3]['content'] = 'Server execution facts (data): '+json.dumps(facts,ensure_ascii=False)
        if original is None:
            original = initial
        assert initial == original, 'An input other than the registered reference index changed'
        assert case['passed'] and case['originals_unchanged'] and case['report']['reported_file_count']==6
        messages = json.loads((OUT/'private'/name/'messages.json').read_text(encoding='utf-8'))
        case.update(file_failure_counts(messages))
        commands = {}
        for message in messages:
            for call in message.get('tool_calls',[]):
                if call['function']['name']=='bash':
                    commands[call['id']] = json.loads(call['function']['arguments'])['command']
            if message['role']=='tool' and message['tool_call_id'] in commands:
                result = json.loads(message['content'])['summary']
                command = commands[message['tool_call_id']]
                if result.get('exit_code')==0 and 'json.load' in command and 'min(' in command and 'max(' in command:
                    code_receipts.append({'case':name,'tool_call_id':message['tool_call_id'],
                        'command_sha256':hashlib.sha256(command.encode()).hexdigest(),
                        'stdout_sha256':hashlib.sha256(result['stdout'].encode()).hexdigest(),
                        'exit_code':0,'partial':result.get('partial',False)})
        assert any(x['case']==name and not x['partial'] for x in code_receipts), 'No actual complete statistics command output'
    measures = ['model_calls','tool_calls','failed_tool_calls','file_not_found','path_failure_tool_calls','file_open_failures','same_argument_repeats',
                'input_tokens','output_tokens','cached_input_tokens','elapsed_seconds','cost_usd']
    totals = {}
    for arm in ['baseline','candidate']:
        selected = [x for x in cases if x['arm']==arm]
        totals[arm] = {'runs':len(selected),'correct_runs':sum(x['passed'] for x in selected)}
        for measure in measures:
            totals[arm][measure] = None if any(x[measure] is None for x in selected) else sum(x[measure] for x in selected)
    changes = {name:None if totals['baseline'][name]==0 else
               100*(totals['baseline'][name]-totals['candidate'][name])/totals['baseline'][name]
               for name in ['model_calls','tool_calls','failed_tool_calls','input_tokens','output_tokens','elapsed_seconds','cost_usd']}
    calls = json.loads((OUT/'provider-usage.json').read_text(encoding='utf-8'))
    pretrial = json.loads((OUT/'pretrial-usage.json').read_text(encoding='utf-8'))
    known = [x for x in calls+pretrial if x.get('usage_source')=='provider']
    unknown = [x for x in calls+pretrial if x.get('usage_source')!='provider']
    assert len(calls)+len(pretrial)<=24
    token_bound = sum(x['input_tokens']+x['output_tokens'] for x in known)+sum(x['reserved_tokens_upper_bound'] for x in unknown)
    cost_bound = sum(x['cost_usd'] for x in known)+sum(x['reserved_cost_upper_bound_usd'] for x in unknown)
    assert token_bound<=300000 and cost_bound<=.10
    summary = {'mode':'Controlled post-compaction prototype A/B; not the full production Harness',
        'case_order':[x['case'] for x in cases],'totals':totals,'observed_reduction_percent':changes,
        'identical_common_inputs_verified':True,'candidate_index_unchanged_after_budget_hardening':True,
        'actual_original_statistics_code_receipts':code_receipts,
        'cohort_model_calls':len(calls),'pretrial_requests_retained':len(pretrial),
        'all_new_provider_requests':len(calls)+len(pretrial),'unknown_usage_requests':len(unknown),
        'known_usage_tokens':sum(x['input_tokens']+x['output_tokens'] for x in known),
        'known_configured_cost_usd':sum(x['cost_usd'] for x in known),
        'all_requests_token_conservative_upper_bound':token_bound,
        'all_requests_configured_cost_conservative_upper_bound_usd':cost_bound,
        'provider_actual_bill_confirmed':False,'production_harness_changed':False,
        'path_error_metric_audit':'Native read error codes alone missed six FileNotFoundError tracebacks in one failed Bash call; totals include both and are reported separately',
        'scope_limitations':['Two pairs only; no statistical/general completion-rate claim',
            'Native read error codes were zero in both arms, but baseline-2 had six missing-file opens inside one failed Bash call; candidate had none',
            'Only exact reference injection was tested against a common recorded lossy summary',
            'Existing production search/version/evidence/authorization behavior was not replaced',
            'Same-argument repeated tool calls were zero in both arms',
            'Provider pretrial client failure and one unknown usage record retained outside the valid cohort']}
    save(OUT/'summary.json',summary)
    print(json.dumps({k:v for k,v in summary.items() if k!='actual_original_statistics_code_receipts'},ensure_ascii=False,indent=2))


if __name__=='__main__':
    main()
