#!/usr/bin/env python3
"""Реальный прогон двух LLM: добавляет экземпляр в запущенный стенд и сохраняет историю."""
import json
import os
from pathlib import Path
import time
import urllib.request

BASE = os.environ.get('ORCHESTRATOR_URL', 'http://127.0.0.1:8092')
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / '.run'
OUT.mkdir(exist_ok=True)

def api(path, body=None):
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request(BASE + path, data=data, headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)

if __name__ == '__main__':
    uid = os.environ.get('INSTANCE_ID')
    if not uid:
        result = api('/api/v1/processes/AiOrchestrator/instances', {
            'businessKey': 'real-qwen-' + str(time.time_ns()),
            'variables': {
                'goal': os.environ.get('GOAL', 'Write a Go function UniqueSorted(values []int) []int. Return unique integers sorted ascending without mutating the input. Return a non-nil empty slice for nil or empty input. Include table-driven tests for negative values, duplicates, nil, empty and input immutability. Keep the answer concise.'),
                'maxRounds': 3,
            },
        })
        uid = result['id']
    print('Instance:', uid, flush=True)
    deadline = time.monotonic() + 3600
    while True:
        instance = api('/api/v1/instances/' + uid)
        if instance['status'] != 'running':
            break
        if time.monotonic() > deadline:
            raise TimeoutError('Process is still running: ' + uid)
        time.sleep(5)
    history = api('/api/v1/instances/' + uid + '/history')
    result = {'instance': instance, 'history': history}
    (OUT / 'e2e-result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    if instance['status'] != 'completed':
        raise RuntimeError('Process status: ' + instance['status'])
    variables = next(r['variables'] for r in reversed(history['records']) if r.get('variables'))
    assert variables['outcome'] in ('accepted', 'needs_human'), variables
    print('Outcome:', variables['outcome'], 'rounds:', variables['round'])
    print('History:', OUT / 'e2e-result.json')
