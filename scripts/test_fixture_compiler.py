"""Resolve a published compiler from both POSIX and Windows npm launchers."""
from pathlib import Path
import json
import os
import subprocess
import tempfile
import unittest
import runpy
import shutil
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]

class PublishedCompiler(unittest.TestCase):
    def test_windows_cmd_shim_resolves_scoped_package(self):
        with tempfile.TemporaryDirectory(prefix='compiler shim ü ') as directory:
            root = Path(directory)
            shim = root/'node_modules/.bin/lmscript.cmd'
            shim.parent.mkdir(parents=True)
            shim.write_text('@echo off\n')
            package = root/'node_modules/@lmctl-ai/lmctl'
            (package/'dist').mkdir(parents=True)
            (package/'package.json').write_text(json.dumps({'name':'@lmctl-ai/lmctl','type':'module','exports':{'./package.json':'./package.json'}}))
            (package/'dist/dsl.js').write_text('export class CompiledDsl { static bytesOf() { return Uint8Array.from([68,83,76,66,0,6]); } }\n')
            (package/'dist/lmscript.js').write_text('export class LMScript { registerFunction(n, fn) { this.capture=fn; } async runAsync() { this.capture({}); return {success:true}; } }\n')
            source=root/'program.lms';source.write_text('program = _lmcode_ schema="host.json";\nlet x=1;\nend _lmcode_\n')
            output=root/'program.dslb'
            env=dict(os.environ,LMSCRIPT_BIN=str(shim));env.pop('LMCODE_COMPILER_MODULE',None)
            result=subprocess.run(['node',str(ROOT/'scripts/compile-fixture.mjs'),str(source),str(output)],env=env,capture_output=True)
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(result.stderr,b'')
            self.assertEqual(output.read_bytes(),b'DSLB\0\6')

class DirectFixtureLaunch(unittest.TestCase):
    def test_python_fixture_check_never_selects_system_bash(self):
        with tempfile.TemporaryDirectory(prefix='fixture argv ') as directory:
            root = Path(directory)
            (root/'scripts').mkdir()
            (root/'bin').mkdir()
            checker = root/'scripts/check-example-fixtures.py'
            shutil.copyfile(ROOT/'scripts/check-example-fixtures.py', checker)
            for rel in ('stdlib/testdata/strlen.dslb', 'testdata/rule-methods/program.dslb'):
                pinned = root/rel; pinned.parent.mkdir(parents=True, exist_ok=True); pinned.write_bytes(b'DSLB\0\6')
            calls = []
            def run(argv, **kwargs):
                calls.append(argv)
                if argv[0] == 'bash':
                    raise AssertionError('Python selected system bash (WSL on Windows)')
                if argv[0] == 'node':
                    (root/argv[-1]).write_bytes(b'DSLB\0\6')
                return subprocess.CompletedProcess(argv, 0)
            with patch('subprocess.run', side_effect=run):
                runpy.run_path(str(checker), run_name='__main__')
            self.assertEqual(sum(argv[0] == 'node' for argv in calls), 4)

if __name__=='__main__':unittest.main()
