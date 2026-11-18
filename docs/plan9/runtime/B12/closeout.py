"""Bounded B12 closeout through native APIs; never manufacture model evidence."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import time

OUT = Path(os.environ.get('ARGUS_B12_RERUN_OUT',
                         'C:/Users/23574/.codex/artifacts/argus-b12-closeout-20261117'))
os.environ['ARGUS_B12_RERUN_OUT'] = str(OUT)
from session_rerun import Rerun, prepare as prepare_same_csv
from session_metrics import model_usage, totals, tool_metrics

MAX_CALLS, MAX_TOKENS, MAX_AMOUNT, RUN_SECONDS = 60, 1500000, .12, 660


class Closeout(Rerun):
    def budget(self, run=None):
        value = self.usage()
        base = self.state['baseline_usage']
        exceeded = (value['request_count'] - base['request_count'] >= MAX_CALLS or
                    value['input_tokens'] + value['output_tokens'] - base['input_tokens'] - base['output_tokens'] >= MAX_TOKENS or
                    float(value['amount']) - float(base['amount']) >= MAX_AMOUNT)
        if exceeded:
            if run:
                self.api.call('POST', '/runs/' + run + '/cancel', key='closeout-budget-' + run)
            self.save('budget-stop', {'run_id': run, 'usage': value, 'at': timestamp()})
            raise RuntimeError('Finite closeout budget reached; all attempts retained')
        return value

    def wait_run(self, ident):
        started = time.monotonic()
        while time.monotonic() - started < RUN_SECONDS:
            value = self.api.call('GET', '/runs/' + ident)
            self.save('run-' + ident, value)
            usage = self.usage()
            print(json.dumps({'run_id': ident, 'status': value['status'],
                              'new_model_calls': usage['request_count'] - self.state['baseline_usage']['request_count']}), flush=True)
            if value['status'] in ['succeeded', 'failed', 'cancelled', 'timed_out']:
                self.save('ledger-current', {'items': self.events()})
                self.save('usage-current', usage)
                if value['status'] != 'succeeded':
                    raise RuntimeError('Native closeout Run ended: ' + value['status'] + '/' + str(value.get('error_code')))
                return value
            self.budget(ident)
            time.sleep(5)
        self.api.call('POST', '/runs/' + ident + '/cancel', key='closeout-timeout-' + ident)
        raise RuntimeError('Native closeout Run exceeded its time window')


def timestamp():
    return datetime.now(timezone.utc).isoformat()


def prepare(v):
    if v.state.get('conversation_id'):
        return
    prepare_same_csv(v)
    v.save('limits', {'calls_max': MAX_CALLS, 'tokens_max': MAX_TOKENS,
                      'amount_max_usd': MAX_AMOUNT, 'per_run_seconds': RUN_SECONDS,
                      'automatic_budget_increase': False, 'model_changed': False,
                      'prior_failures_retained': True, 'prior_csv_reused': True})


def query(v):
    prompt = ('B12 真实收尾复验。仅使用本批已发布的仪表盘查询，不搜索其他资源，不执行主机命令，不修改配置。'
              '对象为 B12-Cache-Host（host:01a1e1dd-d568-724d-8d0b-40098d812b43）与 B12-Redis（monitoring_target:3b24204d-9d9d-5c2a-b3fa-e37d621183a8）。'
              'Host 仪表盘 cee08ea9-0beb-5c7b-a006-e27d11e61d39，只取 CPU使用率、内存使用率、已用内存三个面板；'
              'Redis 仪表盘 6f0cd210-ab49-5d65-be71-f63eb4535beb，只取内存、命令速率、命中率三个面板。'
              '窗口为最近五分钟，分别保留两个任务实际采用的UTC时间范围。'
              '这条消息只要求取数：等待实际查询完成并且原件交付齐全后，用中文简短列出任务、面板、单位和成功/无数据等真实状态即可结束。'
              '下一条消息再分析文件并导出报告。不要先写报告或对原件做统计，不重复获取已经完整返回的完成任务。具体工具流程由你决定。')
    ident = v.send('query', prompt)
    v.wait_run(ident)
    v.save('ledger-after-query', {'items': v.events()})


def analyze(v):
    assert v.api.call('GET', '/runs/' + v.state['query_run'])['status'] == 'succeeded'
    prompt = ('继续同一 B12 场景，直接使用上一步已经完成并交付的两个仪表盘任务和原件，不重新取数、不扫描其他资源。'
              '在 Workspace 实际运行代码计算六个已选面板的统计，按各自资源、面板和实际UTC窗口区分，保留已发布表达式的输出单位；'
              '命令速率、命中率、累计计数不能互换；无数据不按零值处理，不据此断言整个资源异常。'
              '同时实际读取附件 b12-native-redis.csv，计算最后减最初的 total_commands_processed 为 command_delta，used_memory_bytes 最大值为 peak_memory_bytes；'
              'CSV 来自原轮八次真实 INFO，包含监控命令，与本次平台窗口不同，应分别说明而不互相验证业务吞吐。'
              '写入并发布 /workspace/b12-closeout.md，包含可读表格、数据来源与范围、计算过程、原件引用，以及 JSON 代码块：'
              'command_delta、peak_memory_bytes 和 panels 数组；panels 每项包括 resource_id、panel_id、status、unit、from、to、point_count、value_min、value_max、last_value，'
              '无数据项统计为 null、point_count 为0。统计覆盖已返回的全部序列，不把多个设备或进程的不同序列当成同一条线。'
              '最后给出简短中文结论和真实下载引用即可结束。不执行主机命令、不改配置，不反复查相同目录。取证、计算、生成文件的具体方法由你决定。')
    ident = v.send('analysis', prompt, [v.state['csv_file']['id']])
    v.wait_run(ident)
    v.save('ledger-final', {'items': v.events()})


def measure(v):
    events = v.events()
    ids = [v.state[k] for k in ['query_run', 'analysis_run', 'references_run'] if v.state.get(k)]
    usage = model_usage(ids)
    selected = [e for e in events if e.get('run_id') in ids]
    proof = {'conversation_id': v.state['conversation_id'], 'runs': ids,
             'model': totals(usage), 'tools': tool_metrics(selected),
             'compactions': sum(e['event_type'] == 'context_compacted' for e in selected),
             'compaction_failures': sum(e['event_type'] == 'context_compaction_failed' for e in selected),
             'all_attempts_included': True,
             'observational_comparison_only': 'Panel selection and current windows are explicit; this is not a controlled algorithm-only A/B.'}
    v.save('model-call-usage', usage)
    v.save('closeout-metrics', proof)
    print(json.dumps({k: proof[k] for k in ['conversation_id', 'model', 'compactions', 'compaction_failures']}), flush=True)


def references(v):
    assert v.api.call('GET', '/runs/' + v.state['analysis_run'])['status'] == 'succeeded'
    prompt = ('报告的数值、单位、瞬时与实际返回区间以及 CSV 计算已独立核对正确。还需要修正原件引用：'
              'b12-closeout.md 的“原件路径”表用了省略号缩短路径，无法直接定位文件。'
              '请仅依据本会话当前已登记的真实完整文件引用补全六份面板原件、两份 manifest 和 CSV 的完整路径，并核对对应 SHA256；不得猜测或手动拼接不完整 UUID。'
              '保持现有全部统计、单位、时间解释和结论不变，不重新查仪表盘或取数，不执行主机命令，不改配置。'
              '更新同一 /workspace/b12-closeout.md 并真实发布更新后的文件。具体核对、编辑和发布方法由你决定，交付后直接结束。')
    ident = v.send('references', prompt)
    v.wait_run(ident)
    v.save('ledger-final', {'items': v.events()})


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('phase', choices=['prepare', 'query', 'analysis', 'references', 'measure'])
    args = parser.parse_args()
    value = Closeout()
    {'prepare': prepare, 'query': query, 'analysis': analyze, 'references': references, 'measure': measure}[args.phase](value)
