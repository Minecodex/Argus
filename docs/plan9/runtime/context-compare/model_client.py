"""Bounded real provider calls. Keys and hidden reasoning are never serialized."""
import json
import hashlib
import time
import requests

from prepare import OUT, save


def sse_data(lines):
    """Frame bytes before UTF-8 decoding: str.splitlines also splits Unicode NEL."""
    parts = []
    for line in lines:
        if not line:
            if parts:
                yield b'\n'.join(parts).decode('utf-8')
                parts = []
        elif line.startswith(b'data:'):
            parts.append(line[5:].lstrip(b' '))
    if parts:
        yield b'\n'.join(parts).decode('utf-8')


class ModelClient:
    def __init__(self, model, key):
        self.model, self.key = model, key
        self.calls = []
        pretrial = OUT/'pretrial-usage.json'
        self.prior = json.loads(pretrial.read_text(encoding='utf-8')) if pretrial.exists() else []
        self.session = requests.Session()
        self.session.trust_env = False

    def call(self, messages, tools, label):
        body = {'model':self.model['model_id'], 'messages':messages, 'tools':tools,
                'stream':True, 'stream_options':{'include_usage':True}, 'max_tokens':4096,
                'reasoning_effort':'low'}
        history = self.prior+self.calls
        used = sum(c.get('input_tokens',0)+c.get('output_tokens',0) if c.get('usage_source')=='provider'
                   else c['reserved_tokens_upper_bound'] for c in history)
        reserved = len(json.dumps(body, ensure_ascii=False).encode())+4096
        cost = sum(c['cost_usd'] if c.get('usage_source')=='provider' else c['reserved_cost_upper_bound_usd'] for c in history)
        reserve_cost = ((reserved-4096)*.075+4096*.25)/1000000
        if len(history) >= 24 or used+reserved > 300000 or cost+reserve_cost > 0.10:
            raise RuntimeError('Finite comparison budget reached; no additional call dispatched')
        started = time.monotonic()
        call = {'index':len(self.calls)+1,'case':label,'status':'dispatched','max_output_tokens':4096,
                'reserved_tokens_upper_bound':reserved,'reserved_cost_upper_bound_usd':reserve_cost,
                'request_body_hash':hashlib.sha256(json.dumps(body,sort_keys=True).encode()).hexdigest()}
        self.calls.append(call)
        save(OUT/'provider-usage.json', self.calls)
        text, parts, usage, finish, request_id = '', {}, None, None, None
        try:
            with self.session.post(self.model['base_url'].rstrip('/')+'/chat/completions', json=body,
                    headers={'Authorization':'Bearer '+self.key}, stream=True, timeout=(20,120)) as response:
                call['http_status'] = response.status_code
                if response.status_code != 200:
                    raise RuntimeError('Provider HTTP '+str(response.status_code)+'; no automatic request replay')
                for value in sse_data(response.iter_lines(decode_unicode=False)):
                    if time.monotonic()-started > 240:
                        raise RuntimeError('Provider call exceeded finite time window')
                    value = value.strip()
                    if value == '[DONE]':
                        break
                    event = json.loads(value)
                    request_id = event.get('id',request_id)
                    if event.get('usage') is not None:
                        usage = event['usage']
                    for choice in event.get('choices',[]):
                        finish = choice.get('finish_reason') or finish
                        delta = choice.get('delta') or {}
                        text += delta.get('content') or ''
                        for fragment in delta.get('tool_calls') or []:
                            part = parts.setdefault(fragment['index'], {'id':'','type':'function','function':{'name':'','arguments':''}})
                            if fragment.get('id'):
                                part['id'] = fragment['id']
                            f = fragment.get('function') or {}
                            part['function']['name'] += f.get('name') or ''
                            part['function']['arguments'] += f.get('arguments') or ''
            if not usage or not isinstance(usage.get('prompt_tokens'),int) or not isinstance(usage.get('completion_tokens'),int):
                raise RuntimeError('Provider usage missing; stop instead of treating it as zero')
            cached = (usage.get('prompt_tokens_details') or {}).get('cached_tokens')
            if cached is not None and (not isinstance(cached,int) or cached<0 or cached>usage['prompt_tokens']):
                raise RuntimeError('Provider cache usage invalid; stop without executing tools')
            if usage['prompt_tokens']<0 or not 0<=usage['completion_tokens']<=4096:
                raise RuntimeError('Provider usage invalid')
            call.update(input_tokens=usage['prompt_tokens'],output_tokens=usage['completion_tokens'],
                        cached_input_tokens=cached,usage_source='provider',
                        cost_usd=(usage['prompt_tokens']*.075+usage['completion_tokens']*.25)/1000000,
                        finish_reason=finish,provider_request_id=request_id)
            if finish not in ['stop','tool_calls']:
                raise RuntimeError('Provider response incomplete; tools will not execute')
            calls = [parts[i] for i in sorted(parts)]
            message = {'role':'assistant','content':text or None}
            if calls:
                message['tool_calls'] = calls
            call['status'] = 'succeeded'
            return message
        except Exception as error:
            call['status'], call['error'] = 'failed', str(error)
            raise
        finally:
            call['elapsed_seconds'] = round(time.monotonic()-started,3)
            save(OUT/'provider-usage.json',self.calls)
