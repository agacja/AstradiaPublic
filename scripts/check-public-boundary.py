#!/usr/bin/env python3
"""Fail closed on unreviewed files. Run BEFORE pushing a public commit."""
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MANIFEST = ROOT / 'public-files.json'
IGNORED_DIRS = {'.git', 'node_modules', 'target', 'dist', '__pycache__'}
PRIVATE_PATH = re.compile(r'(^|/)(?:server-rust|rust-native|audit|backups?|\.env(?:\..*)?|engine\.rs|strategy\.rs|trader\.rs|runtime\.rs|replay\.rs)(/|$)|(?:calibration|backtest|replay|weights).*(?:\.json|\.csv|\.parquet)$', re.I)
SECRET = re.compile(r'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|sk_live_[A-Za-z0-9]{16,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,})')
PRIVATE_IMPORT = re.compile(r'''(?:from\s*|import\s*|require\s*\(|include_str!\s*\(|include_bytes!\s*\()["'][^"']*(?:server-rust|rust-native|server/lib|server-go/internal/bot|/audit/|/scripts/data/)''')

def files():
    if (ROOT / '.git').exists():
        output = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=ROOT)
        return set(output.decode().strip('\0').split('\0')) - {''}
    return {p.relative_to(ROOT).as_posix() for p in ROOT.rglob('*')
            if p.is_file() and not (set(p.relative_to(ROOT).parts) & IGNORED_DIRS)}

def main():
    approved = json.loads(MANIFEST.read_text())['files']
    observed = files()
    errors = []
    for path in sorted(observed - set(approved) - {'public-files.json'}):
        errors.append(f'unreviewed file: {path}')
    for path in sorted(set(approved) - observed):
        errors.append(f'missing reviewed file: {path}')
    for name in sorted(observed):
        path = ROOT / name
        if path.is_symlink() or not path.resolve().is_relative_to(ROOT):
            errors.append(f'non-local file: {name}')
            continue
        if PRIVATE_PATH.search(name):
            errors.append(f'private or generated path: {name}')
        content = path.read_bytes()
        if name in approved and hashlib.sha256(content).hexdigest() != approved[name]['sha256']:
            errors.append(f'changed file needs publication review: {name}')
        try:
            text = content.decode('utf-8')
        except UnicodeDecodeError:
            if not name.startswith('services/ephemeris/data/ephemeris/') or not name.endswith('.se1'):
                errors.append(f'unapproved binary type: {name}')
            continue
        if SECRET.search(text):
            errors.append(f'possible secret (value redacted): {name}')
        if name.endswith(('.ts', '.tsx', '.js', '.mjs', '.rs', '.go')) and PRIVATE_IMPORT.search(text):
            errors.append(f'private implementation import: {name}')
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print(f'Public boundary passed: {len(approved)} reviewed files; no unapproved files or imports.')
    return 0

if __name__ == '__main__':
    sys.exit(main())
