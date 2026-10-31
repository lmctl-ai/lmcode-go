package lmcode

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invocationTest(t *testing.T, n int) *InvocationResults {
	t.Helper()
	files := make([]string, n)
	for i := range files {
		files[i] = "same"
	}
	r, e := InvocationResultsFromManifest(files, nil)
	mustResult(t, e)
	return r
}
func TestInvocationOrderAndReselection(t *testing.T) {
	r := invocationTest(t, 2)
	reg := NewRegistry()
	b, e := r.Select(1, reg, ResultContract{})
	mustResult(t, e)
	mustResult(t, b.Put("count", Int(1)))
	a, e := r.Select(0, reg, ResultContract{})
	mustResult(t, e)
	mustResult(t, a.Put("count", Int(2)))
	again, e := r.Select(1, reg, ResultContract{})
	mustResult(t, e)
	if again != b || b.JSON() != `{"count":1}` {
		t.Fatal("reselection resets or replaces sink")
	}
	mustResult(t, r.Complete(1))
	mustResult(t, r.Complete(0))
	var out bytes.Buffer
	if r.WriteFrame(&out) != 0 || out.String() != "LMCODE_RETURN_BEGIN\n{\"ok\":true,\"result\":[{\"count\":2},{\"count\":1}],\"parts\":2}\nLMCODE_RETURN_END\n" {
		t.Fatal(out.String())
	}
	if r.WriteFrame(&out) != 4 || strings.Count(out.String(), "LMCODE_RETURN_BEGIN") != 1 {
		t.Fatal("double frame")
	}
}
func TestInvocationSeal(t *testing.T) {
	r := invocationTest(t, 1)
	reg := NewRegistry()
	s, e := r.Select(0, reg, ResultContract{})
	mustResult(t, e)
	mustResult(t, s.Set(Int(3)))
	mustResult(t, r.Complete(0))
	for _, e := range []error{s.Set(Int(4)), s.Put("a", Int(1)), s.SetNull(), s.PutNull("a"), s.Reset()} {
		if e == nil {
			t.Fatal("sealed sink allowed a write")
		}
	}
	if !s.Sealed() {
		t.Fatal("not sealed")
	}
	if _, e = r.Select(0, reg, ResultContract{}); e == nil {
		t.Fatal("selected sealed sink")
	}
	if r.Complete(0) == nil {
		t.Fatal("completed twice")
	}
}
func TestInvocationFirstFailureAndDrain(t *testing.T) {
	r := invocationTest(t, 3)
	reg := NewRegistry()
	_, e := r.Select(0, reg, ResultContract{})
	mustResult(t, e)
	part := 1
	first := NewHostError("first 水")
	first.Locate(4, 2)
	mustResult(t, r.Fail(&part, "run", first))
	part = 2
	first.Message = "mutated"
	mustResult(t, r.Fail(&part, "bogus", nil))
	if _, e = r.Select(2, reg, ResultContract{}); e == nil {
		t.Fatal("started after failure")
	}
	_, e = r.Select(0, reg, ResultContract{})
	mustResult(t, e)
	mustResult(t, r.Complete(0))
	var out bytes.Buffer
	if r.WriteFrame(&out) != 3 {
		t.Fatal(out.String())
	}
	want := "LMCODE_RETURN_BEGIN\n{\"ok\":false,\"phase\":\"run\",\"code\":\"RUN_HOST\",\"message\":\"dsl line 4:2: first 水\",\"line\":4,\"column\":2,\"part\":1}\nLMCODE_RETURN_END\n"
	if out.String() != want {
		t.Fatal(out.String())
	}
}
func TestInvocationMissingAndGlobalFailure(t *testing.T) {
	r := invocationTest(t, 2)
	var out bytes.Buffer
	if r.WriteFrame(&out) != 3 || !strings.Contains(out.String(), `"part":0`) || strings.Contains(out.String(), `"result"`) {
		t.Fatal(out.String())
	}
	r = completedResult(t)
	mustResult(t, r.Fail(nil, "app", NewError(APP_FAIL, "validation")))
	out.Reset()
	if r.WriteFrame(&out) != 4 || strings.Contains(out.String(), `"part"`) || strings.Contains(out.String(), `"result"`) {
		t.Fatal(out.String())
	}
}

type invocationFlushWriter struct {
	bytes.Buffer
	flushes int
	fail    bool
}

func (w *invocationFlushWriter) Flush() error {
	w.flushes++
	if w.fail {
		return io.ErrClosedPipe
	}
	return nil
}
func TestInvocationFlush(t *testing.T) {
	for _, fail := range []bool{false, true} {
		r := completedResult(t)
		w := &invocationFlushWriter{fail: fail}
		want := 0
		if fail {
			want = 4
		}
		if r.WriteFrame(w) != want || w.flushes != 1 {
			t.Fatal("flush lost", w.flushes)
		}
		n := w.Len()
		if r.WriteFrame(w) != 4 || w.Len() != n || w.flushes != 1 {
			t.Fatal("retry after output")
		}
	}
}
func contractTest(t *testing.T, typ string, nullable bool) ResultContract {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"dsl.schema": 2, "result": map[string]any{"type": typ, "nullable": nullable}})
	c, e := ResultContractFromSchema(raw)
	mustResult(t, e)
	return c
}
func TestInvocationContracts(t *testing.T) {
	c := contractTest(t, "IntMap", false)
	s := NewResultSink(c)
	mustResult(t, s.Put("a", Int(1)))
	for _, err := range []error{s.Put("a", Float(1)), s.PutNull("x"), s.SetNull(), s.Set(String("x"))} {
		if err == nil {
			t.Fatal("contract ignored")
		}
	}
	if s.JSON() != `{"a":1}` {
		t.Fatal("non-atomic contract write", s.JSON())
	}
	mustResult(t, s.Reset())
	if s.JSON() != "null" {
		t.Fatal("reset")
	}
	r := invocationTest(t, 2)
	reg := NewRegistry()
	_, e := r.Select(1, reg, c)
	mustResult(t, e)
	if r.Complete(1) == nil {
		t.Fatal("unset nonnullable result")
	}
	var out bytes.Buffer
	r.WriteFrame(&out)
	if !strings.Contains(out.String(), `"part":1`) {
		t.Fatal(out.String())
	}
	r = invocationTest(t, 1)
	_, e = r.Select(0, reg, c)
	mustResult(t, e)
	if _, e = r.Select(0, reg, ResultContract{}); e == nil {
		t.Fatal("changed contract")
	}
	s = NewResultSink(contractTest(t, "FloatList", false))
	mustResult(t, s.Set(Obj("IntList", &IntList{})))
	if s.Set(Obj("StrList", &StrList{})) == nil {
		t.Fatal("empty source type erased")
	}
	j, e := ParseJSON(`[1,2.5]`)
	mustResult(t, e)
	mustResult(t, s.Set(Obj("Json", j)))
	s = NewResultSink(contractTest(t, "Json", true))
	mustResult(t, s.SetNull())
	j, e = ParseJSON(`{"a":[null]}`)
	mustResult(t, e)
	mustResult(t, s.Set(Obj("Json", j)))
	for _, raw := range []string{`{}`, `{"dsl.schema":1}`, `{"dsl.schema":2,"result":null}`, `{"dsl.schema":2,"result":{"type":"null","nullable":false}}`, `{"dsl.schema":2,"result":{"type":"void"}}`, `{"dsl.schema":2,"result":{"type":"int","nullable":1}}`, `{"dsl.schema":2,"result":{"type":"int","x":1}}`, `{"dsl.schema":2,"result":{"type":"int","type":"float"}}`} {
		if _, e := ResultContractFromSchema([]byte(raw)); e == nil {
			t.Fatal("bad contract", raw)
		}
	}
}
func TestInvocationMetadata(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a 水")
	b := filepath.Join(dir, "b")
	raw := `{"version":1,"mode":"builder","parts":[{"index":0,"file":` + resultQuote(a) + `},{"index":1,"file":` + resultQuote(b) + `}]}`
	r, e := InvocationResultsFromMetadata([]byte(raw), []string{b, a})
	mustResult(t, e)
	regs := r.Registrations()
	if regs[0].Index != 1 || regs[1].Index != 0 {
		t.Fatal(regs)
	}
	regs[0].Index = 99
	if r.Registrations()[0].Index != 1 {
		t.Fatal("borrowed registrations")
	}
	for _, change := range [][2]string{{`"version":1`, `"version":2`}, {`"version":1`, `"version":1,"version":1`}, {`"mode":"builder"`, `"mode":"single"`}, {`"index":0`, `"index":0.1`}, {`"index":0`, `"index":9007199254740993`}, {`"index":0`, `"index":0,"contract":{}`}, {resultQuote(a), `"relative"`}, {resultQuote(b), resultQuote(a)}} {
		bad := strings.Replace(raw, change[0], change[1], 1)
		if _, e := InvocationResultsFromMetadata([]byte(bad), []string{a, b}); e == nil {
			t.Fatal(bad)
		}
	}
	for _, files := range [][]string{{a}, {a, a}, {a, b, "extra"}} {
		if _, e := InvocationResultsFromMetadata([]byte(raw), files); e == nil {
			t.Fatal(files)
		}
	}
	for _, bad := range []string{"\xef\xbb\xbf" + raw, "\xff", strings.Replace(raw, `"index":0`, `"index":1e-999`, 1)} {
		if _, e := InvocationResultsFromMetadata([]byte(bad), []string{a, b}); e == nil {
			t.Fatal("invalid raw metadata accepted")
		}
	}
	path := filepath.Join(dir, "manifest.json")
	mustResult(t, os.WriteFile(path, []byte(raw), 0600))
	t.Setenv("LMCODE_INVOCATION_FILE", path)
	_, e = InvocationResultsFromCommand([]string{b, a})
	mustResult(t, e)
	t.Setenv("LMCODE_INVOCATION_FILE", "")
	if _, e = InvocationResultsFromCommand([]string{a}); e == nil {
		t.Fatal("empty env fell back")
	}
}

func TestInvocationContractShapes(t *testing.T) {
	for _, row := range []struct{ typ, good, bad string }{
		{"null", "null", "1"}, {"bool", "true", "0"}, {"int", "2", "2.5"}, {"float", "2.5", `"x"`},
		{"string", `"x"`, "false"}, {"Json", `{"a":[null]}`, "null"},
		{"StrList", `["x"]`, `[null]`}, {"IntList", `[1]`, `[1.5]`}, {"FloatList", `[1,2.5]`, `[true]`},
		{"StrMap", `{"x":"y"}`, `{"x":null}`}, {"IntMap", `{"x":1}`, `{"x":1.5}`}, {"FloatMap", `{"x":1.5}`, `{"x":false}`},
	} {
		t.Run(row.typ, func(t *testing.T) {
			raw := `{"dsl.schema":2,"result":{"type":` + resultQuote(row.typ) + `}}`
			c, e := ResultContractFromSchema([]byte(raw))
			mustResult(t, e)
			s := NewResultSink(c)
			good, e := ParseJSON(row.good)
			mustResult(t, e)
			mustResult(t, s.Set(Obj("Json", good)))
			before := s.JSON()
			bad, e := ParseJSON(row.bad)
			mustResult(t, e)
			if e = s.Set(Obj("Json", bad)); e == nil || !strings.Contains(e.Error(), "result type mismatch at ") {
				t.Fatal("shape accepted", e)
			}
			if s.JSON() != before {
				t.Fatal("contract failure mutated the sink")
			}
		})
	}
}
func TestInvocationFailureValidation(t *testing.T) {
	for _, row := range []struct {
		phase string
		code  int
	}{{"cli", 2}, {"load", 3}, {"run", 3}, {"app", 4}} {
		var out bytes.Buffer
		if code := FailureFrame(&out, row.phase, NewError("CUSTOM", "雪\n")); code != row.code {
			t.Fatal(code)
		}
		if strings.Contains(out.String(), `"part"`) || !strings.Contains(out.String(), `"message":"雪\n"`) {
			t.Fatal(out.String())
		}
	}
	r := invocationTest(t, 1)
	for _, e := range []*Error{{Code: RUN_HOST, Message: "x", HasColumn: true, Column: 1}, {Code: RUN_HOST, Message: "x", HasLocation: true}, {Code: RUN_HOST, Message: "\xff"}} {
		if r.Fail(nil, "run", e) == nil {
			t.Fatal("invalid failure accepted")
		}
	}
	if r.Fail(nil, "unknown", NewHostError("bad")) == nil {
		t.Fatal("invalid phase")
	}
	index := 0
	mustResult(t, r.Fail(&index, "load", NewError(LOAD_BAD_BYTECODE, "bad")))
	var out bytes.Buffer
	if r.WriteFrame(&out) != 3 || strings.Contains(out.String(), `"part"`) {
		t.Fatal("single failure association", out.String())
	}
}
