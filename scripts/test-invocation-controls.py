#!/usr/bin/env python3
"""Production-copy mutations: compilation failures never count as catches."""
import os,pathlib,shutil,subprocess,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
CASES=[
 ('missing seal','result_sink.go','s.sealed = true','s.sealed = false','TestInvocationSeal'),
 ('reused sink','invocation.go','s.sink = NewResultSink(contract)','s.sink = NewResultSink(contract); for _, other := range r.slots { if other.sink != nil && other.sink != s.sink { s.sink = other.sink; break } }','TestInvocationOrderAndReselection'),
 ('wrong part','invocation.go','copy := *index','copy := 0','TestInvocationFirstFailureAndDrain'),
 ('missing flush','invocation.go','err = buffered.Flush()','_ = buffered; err = nil','TestInvocationFlush'),
 ('overwrite first failure','invocation.go','if r.failure != nil {\n\t\treturn nil','if false {\n\t\treturn nil','TestInvocationFirstFailureAndDrain'),
 ('dropped contract','result_contract.go','if c.typ == "" {','if true {','TestInvocationContracts'),
 ('missing completion','invocation.go','if r.slots[i].result == nil {','if r.slots[i].result == nil { r.slots[i].result=&JSON{kind:jsonNull};continue;','TestInvocationMissingAndGlobalFailure'),
 ('repeated output','invocation.go','if r.writing {\n\t\treturn 4','if false {\n\t\treturn 4','TestInvocationFlush'),
 ('registration ignores metadata order','invocation.go','InvocationRegistration{strings.Clone(file), index}','InvocationRegistration{strings.Clone(file), len(r.registrations)}','TestInvocationMetadata'),
]
with tempfile.TemporaryDirectory(prefix='go-invocation-mutants-')as tmp:
 work=pathlib.Path(tmp)/'repo';shutil.copytree(ROOT,work,ignore=shutil.ignore_patterns('.git','bin','__pycache__'))
 originals={f:(work/f).read_text() for _,f,_,_,_ in CASES}
 def run(pattern):
  p=subprocess.run(['go','test','.', '-count=1','-run',pattern],cwd=work,capture_output=True,text=True)
  return p.returncode,p.stdout+p.stderr
 code,out=run('TestInvocation');assert code==0,out
 print('BASELINE PASS',flush=True)
 for name,file,before,after,pattern in CASES:
  for f,text in originals.items():(work/f).write_text(text)
  source=originals[file];assert before in source,('stale anchor',name)
  (work/file).write_text(source.replace(before,after))
  code,out=run(pattern)
  assert code and '--- FAIL:' in out and 'build failed' not in out and 'panic:' not in out,(name,out)
  print('CAUGHT '+name,flush=True)
 print('PASS',len(CASES),'invocation production-copy mutants')
