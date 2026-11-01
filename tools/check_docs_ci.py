#!/usr/bin/env python3
"""Read-only CI entry: immutable baseline and explicit full-scope docs mode.

Stdlib + existing Git/Python only. No fetch, repair, fallback, or authorization
inference. PR checks the checked-out (normally merge) HEAD against base.sha;
push uses before; dispatch requires inputs.base_sha. Missing objects fail closed.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
SHA = re.compile(r'[0-9a-fA-F]{40}')


def select_base(event_name, payload):
    if not isinstance(payload, dict):
        raise ValueError('event JSON must be an object')
    if event_name == 'pull_request':
        pr = payload.get('pull_request')
        base = pr.get('base') if isinstance(pr, dict) else None
        value = base.get('sha') if isinstance(base, dict) else None
    elif event_name == 'push':
        value = payload.get('before')
    elif event_name == 'workflow_dispatch':
        inputs = payload.get('inputs')
        value = inputs.get('base_sha') if isinstance(inputs, dict) else None
    else:
        raise ValueError('unsupported event; only pull_request, push, workflow_dispatch')
    if not isinstance(value, str) or not SHA.fullmatch(value):
        raise ValueError('baseline requires an explicit full 40-hex commit SHA from the event')
    if value == '0' * 40:
        raise ValueError('zero baseline is invalid (new branch: use PR or dispatch with explicit base_sha)')
    return value.lower()


def select_mode(event_name, payload):
    if event_name in {'pull_request', 'push'}:
        # Automatic events cannot carry a manual review declaration.
        return 'maintenance', False
    if event_name != 'workflow_dispatch' or not isinstance(payload, dict):
        raise ValueError('mode selection requires a supported event object')
    inputs = payload.get('inputs', {})
    if not isinstance(inputs, dict):
        raise ValueError('dispatch inputs must be an object')
    mode = inputs.get('mode', 'maintenance')
    if mode not in ('maintenance', 'development'):
        raise ValueError('mode must be maintenance or development')
    declared = inputs.get('reviewed_task_updates', False)
    if type(declared) is bool:
        reviewed = declared
    elif isinstance(declared, str) and declared in ('true', 'false'):
        reviewed = declared == 'true'
    else:
        raise ValueError('reviewed_task_updates must be an explicit boolean')
    if mode == 'maintenance' and reviewed:
        raise ValueError('reviewed_task_updates is valid only in development mode')
    return mode, reviewed


def commit_sha(root, value):
    result = subprocess.run(['git', '-C', str(root), 'rev-parse', '--verify',
                             '--end-of-options', value + '^{commit}'],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise ValueError('commit unavailable in local checkout; no fetch/fallback: ' + value)
    resolved = result.stdout.decode('ascii').strip().lower()
    if not SHA.fullmatch(resolved) or (value != 'HEAD' and resolved != value):
        raise ValueError('baseline must identify a commit object, not a tag or ref')
    return resolved


def run(root, event_name, event_path):
    # JSON-quote untrusted event names/paths; never interpret them as shell code.
    print('EVENT=' + json.dumps(event_name), flush=True)
    print('SCOPE=all maintained Markdown (including unchanged sources)', flush=True)
    try:
        head = commit_sha(root, 'HEAD')
        print('HEAD=' + head, flush=True)
        if not event_path:
            raise ValueError('event JSON path is required')
        payload = json.loads(Path(event_path).read_text(encoding='utf-8'))
        base = select_base(event_name, payload)
        mode, reviewed = select_mode(event_name, payload)
        print('MODE=' + mode + '; REVIEWED_TASK_UPDATES=' + str(reviewed).lower() +
              ' (caller declaration only, not verified authorization or acceptance)', flush=True)
        print('BASE=' + base, flush=True)
        base = commit_sha(root, base)
        if base == head:
            raise ValueError('self-baseline rejected; base must differ from actual checked-out HEAD')
        command = [sys.executable, '-B', str(root / 'tools/check_docs.py'),
                   '--base', base, '--mode', mode]
        if reviewed:
            command.append('--reviewed-task-updates')
        print('COMMAND=' + json.dumps(command), flush=True)
        result = subprocess.run(command, cwd=root)
        code = result.returncode if result.returncode >= 0 else 128 - result.returncode
    except (ValueError, OSError, UnicodeError) as exc:
        print('RESULT=FAILED exit=2 reason=' + json.dumps(str(exc)), flush=True)
        return 2
    print('RESULT=' + ('PASSED' if code == 0 else 'FAILED') + ' exit=' + str(code), flush=True)
    return code


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--event-name', default=os.environ.get('GITHUB_EVENT_NAME'))
    parser.add_argument('--event-path', default=os.environ.get('GITHUB_EVENT_PATH'))
    args = parser.parse_args(argv)
    return run(ROOT, args.event_name, args.event_path)


if __name__ == '__main__':
    sys.exit(main())
