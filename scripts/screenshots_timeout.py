#!/usr/bin/env python3
"""Снять настоящий таймаут и выполнить задачу через стандартную консоль."""
import json
import os
import time
from screenshots import BASE, OUT, cdp, evaluate, get, save, shot


def wait(expression):
    for _ in range(100):
        if evaluate(expression):
            return
        time.sleep(.1)
    raise RuntimeError('UI did not become ready: ' + expression)


if __name__ == '__main__':
    uid = os.environ['INSTANCE_ID']
    task = next(t for t in get(BASE + '/api/v1/tasks?instanceId=' + uid)['tasks'] if t['element_id'] == 'HumanReview')
    key = task['business_key']
    cdp('Emulation.setDeviceMetricsOverride', {'width': 1800, 'height': 1100, 'deviceScaleFactor': 1, 'mobile': False})
    shot('04-timeout.png', '/ui/history/?instanceId=' + uid,
         'document.querySelectorAll("#canvas .djs-element").length > 3')
    shot('05-human-task.png', '/ui/#/tasks?user=reviewer&groups=reviewers',
         'document.querySelector("#tasks-open-count").textContent !== "0"')
    find = 'const c=[...document.querySelectorAll("li.card")].find(c=>c.textContent.includes(' + json.dumps(key) + '));if(!c)throw Error("Task missing");'
    evaluate('(()=>{' + find + '[...c.querySelectorAll("button")].find(b=>b.textContent==="Взять").click()})()')
    wait('[...document.querySelectorAll("li.card")].some(c=>c.textContent.includes(' + json.dumps(key) + ')&&c.textContent.includes("Выполнить"))')
    evaluate('(()=>{' + find + '[...c.querySelectorAll("button")].find(b=>b.textContent==="Выполнить").click()})()')
    wait('document.querySelector("#complete").open')
    evaluate('(()=>{const b=[...document.querySelectorAll("#complete-editor button")].find(b=>b.textContent.includes("JSON"));b.click()})()')
    decision = {'humanApproved': False, 'humanComment': 'AI превысил срок итерации. Результат отклонён, требуется отдельный повтор после проверки задания.'}
    evaluate('document.querySelector("#complete-editor textarea").value=' + json.dumps(json.dumps(decision, ensure_ascii=False)))
    save('06-human-decision.png')
    evaluate('document.querySelector("#complete-form").requestSubmit()')
    wait('!document.querySelector("#complete").open')
    for _ in range(100):
        if get(BASE + '/api/v1/instances/' + uid)['status'] == 'completed':
            break
        time.sleep(.1)
    else:
        raise RuntimeError('Process did not finish after human decision')
    shot('07-human-completed.png', '/ui/history/?instanceId=' + uid,
         'document.querySelectorAll("#canvas .djs-element").length > 3')
