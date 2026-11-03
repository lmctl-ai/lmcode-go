#!/usr/bin/env python3
"""Build both examples with the selected compiler and inspect the actual wire header."""
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
(root/'bin').mkdir(exist_ok=True)
subprocess.run([sys.executable, str(root/'scripts/test_fixture_compiler.py')], check=True)
subprocess.run(['bash', 'scripts/compile-examples.sh'], cwd=root, check=True)
for name in ('rule-entries', 'greeter'):
    data = (root/'bin'/(name+'.dslb')).read_bytes()
    assert data[:6] == b'DSLB\x00\x06', (name, 'expected v6 fixture')
print('PASS both compiler-generated example fixtures')
