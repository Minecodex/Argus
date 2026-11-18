"""Bounded real-model B12 rerun; retain failed attempts and provider usage."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

from context_runtime_verify import Verification, ROOT
from common.api import Api

OUT = Path(os.environ.get('ARGUS_B12_RERUN_OUT', 'C:/Users/23574/.codex/artifacts/argus-b12-rerun-20261116-01'))
if os.environ.get('ARGUS_B12_RERUN_ATTEMPT'):
    OUT = OUT / os.environ['ARGUS_B12_RERUN_ATTEMPT']
MODEL = '01a1e1e4-562e-7e1f-b123-2656935a8dfa'


class Rerun(Verification):
    def __init__(self):
        self.id = 'argus-b12-rerun-' + hashlib.sha256(str(OUT).encode()).hexdigest()[:16]
        self.out = OUT
        self.private = OUT / 'private'
        self.resources = OUT / 'state.json'
        self.private.mkdir(parents=True, exist_ok=True)
        user = subprocess.check_output(['whoami'], text=True).strip()
        subprocess.run(['icacls', str(self.private), '/inheritance:r', '/grant:r', user + ':(OI)(CI)F', '*S-1-5-18:(OI)(CI)F'], capture_output=True, check=True)
        path = self.private / 'session.json'
        if not path.exists():
            path.write_text(json.dumps({'origin': 'https://argus.dev', 'csrf': '', 'user': {}, 'cookies': []}))
        self.api = Api(self, path, ROOT / 'deploy/.cache/argus-trust-bundle.pem',
                       {'enterprise_username': 'kakj', 'password': os.environ['ARGUS_PLAN9_PASSWORD']})
        self.api.login()
        self.state = json.loads(self.resources.read_text(encoding='utf-8')) if self.resources.exists() else {'model_id': MODEL}

    def budget(self, run=None):
        usage = self.usage()
        base = self.state['baseline_usage']
        over = (usage['request_count'] - base['request_count'] >= 45 or
                float(usage['amount']) - float(base['amount']) >= .12 or
                usage['input_tokens'] + usage['output_tokens'] - base['input_tokens'] - base['output_tokens'] >= 1000000)
        if over:
            if run:
                self.api.call('POST', '/runs/' + run + '/cancel', key='finite-rerun-stop-' + run)
            raise RuntimeError('Rerun observation limit reached; all attempts retained')
        return usage

    def wait_run(self, ident):
        started = time.monotonic()
        while time.monotonic() - started < 660:
            value = self.api.call('GET', '/runs/' + ident)
            self.save('run-' + ident, value)
            if value['status'] in ['succeeded', 'failed', 'cancelled', 'timed_out']:
                self.save('ledger-current', {'items': self.events()})
                self.save('usage-current', self.usage())
                print('Terminal Run ' + ident + ': ' + value['status'], flush=True)
                if value['status'] != 'succeeded':
                    raise RuntimeError('Real Run did not succeed: ' + value['status'])
                return value
            self.budget(ident)
            print('Run ' + ident + ': ' + value['status'] + '; charged model requests=' + str(self.usage()['request_count'] - self.state['baseline_usage']['request_count']), flush=True)
            time.sleep(5)
        self.api.call('POST', '/runs/' + ident + '/cancel', key='time-window-stop-' + ident)
        raise RuntimeError('Bounded Run time window reached')


def prepare(v):
    if v.state.get('conversation_id'):
        return
    v.state['baseline_usage'] = v.usage()
    chat = v.api.call('POST', '/conversations', {'title': 'B12 重新执行 · 工具成功率与Token复测', 'selected_model_id': MODEL}, key='fresh-measured-chat', expected=201)
    v.state['conversation_id'] = chat['id']
    v.save_state()
    v.save('limits', {'calls_max': 45, 'tokens_max': 1000000, 'amount_max_usd': .12, 'per_run_seconds': 660,
                      'production_changed': False, 'model_changed': False, 'native_quota_increased': False})
    source_chat = '01a1e1f0-4809-78f5-bb8b-9b7b96e35a44'
    source_file = 'e28a2d14-b644-4882-8312-7d0fa2252045'
    response = v.api.http.get(v.api.origin + '/api/v1/conversations/' + source_chat + '/workspace/files/' + source_file + '/content', timeout=45)
    response.raise_for_status()
    body = response.content
    digest = hashlib.sha256(body).hexdigest()
    assert len(body) == 535 and digest == 'a0dd55384c6ed1bc0ca6cda4ef9a2ed7a138a27fba97259ec0b383e14bda6a76'
    upload = v.api.call('POST', '/conversations/' + chat['id'] + '/workspace/uploads', {'name': 'b12-native-redis.csv', 'byte_size': len(body)}, key='identical-original-csv', expected=201)
    response = v.api.http.put(v.api.origin + '/api/v1/conversations/' + chat['id'] + '/workspace/uploads/' + upload['id'] + '/content', data=body,
                             headers={'X-CSRF-Token': v.api.csrf, 'Idempotency-Key': 'b12-rerun-content-' + upload['id'], 'Content-Type': 'application/octet-stream'}, timeout=90)
    assert response.status_code in [200, 201]
    v.state['csv_file'] = response.json()
    v.save_state()
    print('Prepared real chat ' + chat['id'], flush=True)


def query(v):
    baseline = json.loads((OUT / 'historical-baselines.json').read_text(encoding='utf-8'))
    ident = v.send('query', baseline['query']['prompt'])
    v.wait_run(ident)
    v.save('ledger-after-query', {'items': v.events()})


def analyze(v):
    prior = v.api.call('GET', '/runs/' + v.state['query_run'])
    assert prior['status'] in ['succeeded', 'failed', 'cancelled', 'timed_out']
    prompt = ('继续 B12 真实验收，仅针对同一 B12-Cache-Host 和 B12-Redis，使用本会话上一步已取得的平台指标与默认仪表盘结果，'
              '不要重新取数或扫描其他资源。附件 b12-native-redis.csv 是此 Redis 的8次真实 INFO，包含监控命令，不能将总命令数当纯业务吞吐。'
              '必须实际在 Workspace 执行代码读取附件，计算最后减最初的 total_commands_processed 为 command_delta，used_memory_bytes 最大值为 peak_memory_bytes。'
              '写入 /workspace/b12-analysis.md，包含源文件、计算过程、上一步真实观测结论及范围/时间/单位、这两个原始整数字段的 JSON 代码块，'
              '并用 workflow.publish_file 发布报告。CSV 与平台指标窗口不同，应分别说明。不要执行任何主机命令，不改配置。充分取证后直接交付，不反复查相同目录。')
    if prior.get('error_code') == 'CONTEXT_COMPACTION_FAILED':
        # This is now the same real failure preceding the original B12 CSV task.
        # Send its original instruction, without a synthesized answer or script.
        baseline = json.loads((OUT / 'historical-baselines.json').read_text(encoding='utf-8'))
        prompt = baseline['analysis']['prompt']
    ident = v.send('analysis', prompt, [v.state['csv_file']['id']])
    v.wait_run(ident)
    v.save('ledger-final', {'items': v.events()})
    v.save('usage-final', v.usage())


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('phase', choices=['prepare', 'query', 'analysis'])
    args = parser.parse_args()
    value = Rerun()
    {'prepare': prepare, 'query': query, 'analysis': analyze}[args.phase](value)
