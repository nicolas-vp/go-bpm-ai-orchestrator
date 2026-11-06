#!/usr/bin/env python3
"""Оригинальные скриншоты работающего AI Orchestrator через Chrome DevTools, без сторонних пакетов."""
import base64
import json
import os
from pathlib import Path
import socket
import struct
import time
import uuid
import urllib.request

ROOT=Path(__file__).resolve().parents[1]
BASE=os.environ.get('ORCHESTRATOR_URL','http://127.0.0.1:8092')
DEBUG=os.environ.get('CHROME_DEBUG','http://127.0.0.1:9231')
OUT=ROOT/'docs/images'
OUT.mkdir(parents=True,exist_ok=True)

def get(url):
    with urllib.request.urlopen(url,timeout=10) as r:return json.load(r)

def cdp(method,params=None):
    target=next(t for t in get(DEBUG+'/json/list') if t['type']=='page')
    from urllib.parse import urlsplit
    u=urlsplit(target['webSocketDebuggerUrl'])
    s=socket.create_connection((u.hostname,u.port),timeout=20)
    s.sendall((f'GET {u.path} HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n').encode())
    h=b''
    while not h.endswith(b'\r\n\r\n'):h+=s.recv(1)
    assert b'101' in h,h
    payload=json.dumps({'id':1,'method':method,'params':params or {}}).encode()
    mask=os.urandom(4)
    head=bytes([0x81,0x80|len(payload)]) if len(payload)<126 else b'\x81\xfe'+struct.pack('!H',len(payload))
    s.sendall(head+mask+bytes(v^mask[i%4] for i,v in enumerate(payload)))
    def read(n):
        b=b''
        while len(b)<n:
            chunk=s.recv(n-len(b))
            if not chunk:raise RuntimeError('WebSocket закрыт')
            b+=chunk
        return b
    while True:
        h=read(2);n=h[1]&127
        if n==126:n=struct.unpack('!H',read(2))[0]
        elif n==127:n=struct.unpack('!Q',read(8))[0]
        msg=json.loads(read(n))
        if msg.get('id')==1:
            s.close()
            if 'error' in msg:raise RuntimeError(msg['error'])
            return msg['result']

def evaluate(expression):
    r=cdp('Runtime.evaluate',{'expression':expression,'returnByValue':True})
    if 'exceptionDetails' in r:raise RuntimeError(r)
    return r['result'].get('value')

def shot(name,path,ready='document.readyState === "complete"'):
    cdp('Page.navigate',{'url':BASE+path})
    for _ in range(100):
        if evaluate(ready):break
        time.sleep(.1)
    else:raise RuntimeError('Страница не готова: '+path)
    time.sleep(.3)
    error=evaluate('document.querySelector("#error:not([hidden])")?.textContent || ""')
    assert not error,error
    save(name)

def save(name):
    image=cdp('Page.captureScreenshot',{'format':'png','captureBeyondViewport':False})
    (OUT/name).write_bytes(base64.b64decode(image['data']))
    print('Скриншот:',name,flush=True)

if __name__ == '__main__':
    cdp('Emulation.setDeviceMetricsOverride',{'width':1600,'height':1000,'deviceScaleFactor':1,'mobile':False})
    shot('01-processes.png','/ui/','document.querySelectorAll("#processes-body tr").length === 1')
    uid=os.environ.get('INSTANCE_ID')
    if uid:
        shot('02-history.png','/ui/history/?instanceId='+uid,'document.querySelectorAll("#canvas .djs-element").length > 3')
        pos=evaluate('(()=>{const r=document.querySelector("[data-element-id=Review] .djs-visual").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()')
        for kind in ['mousePressed','mouseReleased']:
            cdp('Input.dispatchMouseEvent',{'type':kind,'x':pos['x'],'y':pos['y'],'button':'left','clickCount':1})
        time.sleep(.5)
        evaluate('document.querySelector("#panel-body").scrollTop = document.querySelector("#panel-body").scrollHeight')
        time.sleep(.2)
        save('03-review.png')
