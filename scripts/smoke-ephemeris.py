#!/usr/bin/env python3
"""Exercise the real public astronomy binary with ephemeral local credentials."""
import io
import json
import math
from pathlib import Path, PurePosixPath
import secrets
import socket
import subprocess
import sys
import tarfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
SERVICE = ROOT / 'services/ephemeris'
BINARY = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else SERVICE / 'target/debug/astradia-ephemeris'
with socket.socket() as listener:
    listener.bind(('127.0.0.1', 0))
    port = listener.getsockname()[1]
secret = secrets.token_urlsafe(36)
env = {'EPHEMERIS_BIND': f'127.0.0.1:{port}', 'EPHEMERIS_SERVICE_SECRET': secret,
       'ASTRADIA_EPHEMERIS_PATH': str(SERVICE / 'data/ephemeris')}

def request(path, body=None, authorized=False):
    headers = {'Content-Type': 'application/json'}
    if authorized:
        headers['x-ephemeris-secret'] = secret
    req = urllib.request.Request(f'http://127.0.0.1:{port}{path}',
        data=None if body is None else json.dumps(body).encode(), headers=headers)
    try:
        response = urllib.request.urlopen(req, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, response.read()

process = subprocess.Popen([str(BINARY)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
try:
    for _ in range(100):
        if process.poll() is not None:
            raise RuntimeError('Astronomy fixture exited before readiness')
        try:
            if request('/health')[0] == 200:
                break
        except urllib.error.URLError:
            pass
        time.sleep(0.05)
    else:
        raise RuntimeError('Astronomy fixture did not become ready')
    sample = {'timestampMs': 1767225600000, 'latitude': 0, 'longitude': 0}
    assert request('/v1/ephemeris', sample)[0] == 401
    status, raw = request('/v1/ephemeris', sample, True)
    assert status == 200
    positions = json.loads(raw)
    assert len(positions['planets']) == 10
    assert all(math.isfinite(p['longitude']) and 0 <= p['longitude'] < 360 for p in positions['planets'])
    assert positions['houseCusps'] is not None
    assert request('/v1/ephemeris', {**sample, 'latitude': 100}, True)[0] == 400
    assert request('/v1/ephemeris', {'timestampMs': sample['timestampMs'], 'latitude': 0}, True)[0] == 400
    assert request('/v1/ephemeris', {**sample, 'asset': 'DEMO'}, True)[0] == 422
    status, archive = request('/source')
    assert status == 200
    expected = {'Cargo.toml', 'Cargo.lock', 'build.rs', 'src/main.rs', 'src/protocol.rs',
                'LICENSE', 'SWISS-LICENSE', 'PROTOCOL-LICENSE', 'README.md', 'Dockerfile'}
    expected |= {'data/ephemeris/' + name for name in ['LICENSE', 'seas_18.se1', 'seasnam.txt',
        'sefstars.txt', 'seleapsec.txt', 'semo_18.se1', 'seorbel.txt', 'sepl_18.se1']}
    with tarfile.open(fileobj=io.BytesIO(archive), mode='r:gz') as tar:
        seen = set()
        for member in tar.getmembers():
            parts = PurePosixPath(member.name).parts
            assert member.isfile() and parts[0] == 'astradia-ephemeris' and '..' not in parts
            name = '/'.join(parts[1:])
            assert name in expected and name not in seen
            assert tar.extractfile(member).read() == (SERVICE / name).read_bytes()
            seen.add(name)
        assert seen == expected
    print('Astronomy smoke checks passed: health, auth, calculations, validation, and exact public source archive.')
finally:
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
