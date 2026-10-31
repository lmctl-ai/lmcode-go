#!/usr/bin/env python3
"""Check the fixture host's actual exit status and terminal stdout bytes."""
import json,pathlib,subprocess,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
DIR=ROOT/'conformance/return-vectors'
binary=pathlib.Path(sys.argv[1]).resolve()
manifest=json.loads((DIR/'manifest.json').read_text())
for case in manifest['vectors']:
 result=subprocess.run([str(binary),str(DIR/case['bytecode']),str(case['repeat'])],capture_output=True)
 expected=(DIR/case['expect']['file']).read_bytes()
 assert (result.returncode,result.stdout)==(case['expect']['exit'],expected),(case['name'],result.returncode,result.stdout,result.stderr)
 if case['expect']['exit']==0:
  assert result.stdout.endswith(b'LMCODE_RETURN_END\n') and result.stdout.count(b'\n')==3,case['name']
 else:assert b'dsl line ' in result.stderr,case['name']
 print('PASS terminal return '+case['name'])
result=subprocess.run([str(binary),str(DIR/'entry-sink.dslb'),'1','--app-fail'],capture_output=True)
assert result.returncode==4 and result.stdout==b'LMCODE_RETURN_BEGIN\n{"ok":false,"phase":"app","code":"APP_FAIL","message":"native validation failed"}\nLMCODE_RETURN_END\n',(result.returncode,result.stdout)
print('PASS application failure has no partial result')
