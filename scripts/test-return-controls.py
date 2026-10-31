#!/usr/bin/env python3
"""Mutate independent production copies; no worktree edits or altered assertions."""
import os, pathlib, shutil, subprocess, tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
CASES=[
 ('buffered writer failure','invocation.go',[('err = buffered.Flush()', '_ = buffered; err = nil')],'TestResultFlushFailure'),
 ('shortest formatter','result_sink.go',[("strconv.FormatFloat(value, 'g', -1, 64)","FormatG15(value)")],'TestResultFrameNumericAndStrings'),
 ('JSON source alias','result_sink.go',[("return copyResultJSON(j, path, map[*JSON]bool{})","return j, nil")],'TestResultSinkOwnershipAndLifecycle'),
 ('arena teardown','registry.go',[("func (r *Registry) endRun() {","func (r *Registry) endRun() { if r.runState.resultSink != nil { r.runState.resultSink.Reset() }")],'TestResultSinkOwnershipAndLifecycle'),
 ('reset between calls','registry.go',[("func (r *Registry) beginRun() error {","func (r *Registry) beginRun() error { if r.runState.resultSink != nil { r.runState.resultSink.Reset() }")],'TestResultSinkOwnershipAndLifecycle'),
 ('missing sink','result_sink.go',[("return None(), NewHostError(\"lmcode return: no result sink\")","return None(), nil")],'TestResultMissingAndReboundSink'),
 ('raw numeric validation','result_sink.go',[("func resultRawNumber(raw, path string) error {","func resultRawNumber(raw, path string) error { return nil")],'TestResultRawNumbers'),
 ('integer bound','result_sink.go',[("const resultSafeInteger int64 = 9007199254740991","const resultSafeInteger int64 = 9007199254740992")],'TestResultAtomicErrorsAndOrder'),
 ('underflow','result_sink.go',[("if j.floating == 0 {","if false {")],'TestResultRawNumbers'),
 ('UTF8 values','result_sink.go',[("if !utf8.ValidString(value) {","if false {")],'TestResultAtomicErrorsAndOrder'),
 ('UTF8 keys','result_sink.go',[("if !utf8.ValidString(key) {","if false {")],'TestResultAtomicErrorsAndOrder'),
 ('cycle refusal','result_sink.go',[("return nil, resultInvalid(path, \"cycle\")","return &JSON{kind:jsonNull}, nil")],'TestResultCyclesAndHandles'),
 ('atomic set','result_sink.go',[("owned, err := copyResultValue(value, \"\")\n\tif err != nil {","owned, err := copyResultValue(value, \"\")\n\tif err != nil { s.root=nil;")],'TestResultFailedSetIsAtomic'),
 ('field overwrite order','result_sink.go',[("if next.object[i].key == key {","if false {")],'TestResultAtomicErrorsAndOrder'),
 ('global sink','result_sink.go',[("const resultSafeInteger", "var sharedResultSink *ResultSink\nconst resultSafeInteger"),("r.runState.resultSink","sharedResultSink")],'TestResultDistinctSinks'),
 ('success after entry failure','conformance/returns.go',[('_ = results.Fail(&index, "run", err)','_ = index')],'TestCanonicalReturnVectors/entry-error'),
 ('success after app validation failure','conformance/returns.go',[('_ = results.Fail(nil, "app", lmcode.NewError(lmcode.APP_FAIL, "native validation failed"))','_ = 0')],'TestCanonicalReturnVectors'),
]

with tempfile.TemporaryDirectory(prefix='wp30b-mutants-') as temporary:
 work=pathlib.Path(temporary)/'repo'
 shutil.copytree(ROOT,work,ignore=shutil.ignore_patterns('.git','bin','dist','__pycache__'))
 originals={name:(work/name).read_text() for _,name,_,_ in CASES}
 env=os.environ.copy();env['CGO_ENABLED']='0'
 def run(test):
  result=subprocess.run(['go','test','.', '-count=1','-run',test],cwd=work,env=env,capture_output=True,timeout=90)
  return result.returncode,(result.stdout+result.stderr).decode(errors='replace')
 code,output=run('TestResult|TestReturn|TestCanonicalReturn')
 assert code==0,output
 print('BASELINE PASS',flush=True)
 for label,name,changes,test in CASES:
  for file,source in originals.items():(work/file).write_text(source)
  source=originals[name]
  for before,after in changes:
   assert before in source,('missing anchor',label,before)
   source=source.replace(before,after)
  (work/name).write_text(source)
  code,output=run(test)
  assert code!=0,('SURVIVED',label)
  assert '--- FAIL:' in output and 'build failed' not in output and 'panic:' not in output,(label,output)
  print('CAUGHT '+label,flush=True)
 print('PASS '+str(len(CASES))+' production-copy mutants',flush=True)
