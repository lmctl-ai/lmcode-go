#!/usr/bin/env python3
"""Build both examples with the selected compiler and inspect the actual wire header."""
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
(root/'bin').mkdir(exist_ok=True)
subprocess.run([sys.executable, str(root/'scripts/test_fixture_compiler.py')], check=True)
for name, source in [('rule-entries', 'examples/rule-entries/rules.lms'), ('greeter', 'examples/greeter/build.lms')]:
    subprocess.run(['node', 'scripts/compile-fixture.mjs', source, 'bin/'+name+'.dslb'], cwd=root, check=True)
for name, source, pinned in [
    ('strlen', 'stdlib/testdata/build.lms', 'stdlib/testdata/strlen.dslb'),
    ('rule-methods', 'testdata/rule-methods/program.lms', 'testdata/rule-methods/program.dslb'),
]:
    generated = 'bin/'+name+'-source.dslb'
    subprocess.run(['node', 'scripts/compile-fixture.mjs', source, generated], cwd=root, check=True)
    if (root/generated).read_bytes() != (root/pinned).read_bytes():
        raise SystemExit('lmcode fixture source no longer compiles to its pinned bytes: '+source)
for name in ('rule-entries', 'greeter'):
    data = (root/'bin'/(name+'.dslb')).read_bytes()
    assert data[:6] == b'DSLB\x00\x06', (name, 'expected v6 fixture')
print('PASS both compiler-generated example fixtures')
