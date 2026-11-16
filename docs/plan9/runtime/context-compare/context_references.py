"""Business-neutral prototype: exact trusted references survive lossy summaries.

Providers supply this metadata explicitly. Never infer references/authority from
model text, external tool payloads or a natural-language compaction summary.
"""
from dataclasses import asdict, dataclass
import hashlib
import json
from pathlib import PurePosixPath
import re


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False,
                                     separators=(',', ':')).encode()).hexdigest()


@dataclass(frozen=True)
class FileReference:
    file_id: str
    workspace_id: str
    path: str
    sha256: str
    byte_size: int
    source_result_ref: str
    media_type: str = 'application/octet-stream'

    def __post_init__(self):
        path = PurePosixPath(self.path)
        if (not self.file_id or not self.workspace_id or not self.source_result_ref
                or '\0' in self.path or '\\' in self.path or '..' in path.parts
                or not self.path.startswith('/workspace/') or str(path) != self.path
                or len(self.path) > 4096 or not re.fullmatch('[a-f0-9]{64}', self.sha256)
                or self.byte_size < 0):
            raise ValueError('INVALID_REFERENCE_METADATA')


class ReferenceCatalog:
    """Immutable identity, explicit whole-record budgets and current authority."""
    def __init__(self, authorization_scope, workspace_id, through_sequence):
        if not authorization_scope or not workspace_id or through_sequence < 0:
            raise ValueError('INVALID_CONTEXT_IDENTITY')
        self.scope = authorization_scope
        self.workspace = workspace_id
        self.through = through_sequence
        self._files = {}

    def register(self, reference, *, authorization_scope, source_sequence):
        if authorization_scope != self.scope or reference.workspace_id != self.workspace:
            raise ValueError('REFERENCE_SCOPE_MISMATCH')
        if source_sequence > self.through or source_sequence < 0:
            raise ValueError('REFERENCE_OUTSIDE_CONTEXT_BOUNDARY')
        previous = self._files.get(reference.file_id)
        if previous is not None and previous != reference:
            raise ValueError('REFERENCE_IDENTITY_CONFLICT')
        self._files[reference.file_id] = reference

    def project(self, *, authorization_scope, workspace_id, max_items=64, max_bytes=16384):
        # A permission/workspace change invalidates old references. No path rebinding.
        if authorization_scope != self.scope or workspace_id != self.workspace:
            return {'available': False, 'reason': 'REFERENCE_SCOPE_CHANGED', 'files': []}
        if max_items < 0 or max_bytes < 512:
            raise ValueError('INVALID_REFERENCE_BUDGET')
        value = {'schema_version': 'argus.context_references.prototype/v1',
                 'authorization_scope': self.scope, 'workspace_id': self.workspace,
                 'through_sequence': self.through, 'available': True, 'complete': True,
                 'omitted_count': 0, 'files': [],
                 'semantics': 'Exact registered references, not proof of file contents. '
                              'Use read/bash to inspect. Current authorization and content '
                              'integrity are checked at access; summaries are historical data.'}
        for reference in self._files.values():
            candidate = {**value, 'files': value['files'] + [asdict(reference)]}
            if len(candidate['files']) > max_items or len(json.dumps(candidate).encode()) > max_bytes - 128:
                break
            value = candidate
        value['omitted_count'] = len(self._files) - len(value['files'])
        value['complete'] = value['omitted_count'] == 0
        if not value['complete']:
            value['recovery'] = 'Read the original registered result metadata; this index is partial.'
        value['context_hash'] = digest(value)
        if len(json.dumps(value).encode()) > max_bytes:
            if not value['files']:
                raise ValueError('REFERENCE_BUDGET_TOO_SMALL')
            return self.project(authorization_scope=authorization_scope,workspace_id=workspace_id,
                                max_items=len(value['files'])-1,max_bytes=max_bytes)
        return value


class InvocationLedger:
    """Deduplicate retry identity; equal args alone never imply equal intent."""
    def __init__(self):
        self.rows = {}

    def begin(self, invocation_id, arguments, scope):
        identity = digest({'arguments': arguments, 'scope': scope})
        if invocation_id in self.rows:
            row = self.rows[invocation_id]
            if row['identity'] != identity:
                raise ValueError('INVOCATION_IDENTITY_CONFLICT')
            return row
        self.rows[invocation_id] = {'identity': identity, 'status': 'pending', 'result': None}
        return None

    def finish(self, invocation_id, result, status='succeeded'):
        if status not in ['succeeded', 'failed', 'unknown']:
            raise ValueError('INVALID_EXECUTION_STATE')
        row = self.rows[invocation_id]
        if row['status'] != 'pending':
            raise ValueError('INVOCATION_ALREADY_TERMINAL')
        row['status'], row['result'] = status, result


def verify_content(reference, content):
    if len(content) != reference.byte_size or hashlib.sha256(content).hexdigest() != reference.sha256:
        raise ValueError('REFERENCE_CONTENT_CHANGED')
