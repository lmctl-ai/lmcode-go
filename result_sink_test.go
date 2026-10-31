package lmcode

import (
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestReturnSignatures(t *testing.T) {
	r := NewRegistry()
	n := 0
	for _, sig := range r.StdSignatures() {
		if strings.HasPrefix(sig, "fn lmcode_return_") {
			n++
		}
	}
	if n != 24 {
		t.Fatalf("return signatures: got %d, want 24", n)
	}
}

func mustResult(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func resultError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
	if e, ok := err.(*Error); !ok || e.Code != RUN_HOST {
		t.Fatalf("not RUN_HOST: %v", err)
	}
}
func TestResultSinkOwnershipAndLifecycle(t *testing.T) {
	r := NewRegistry()
	s := &ResultSink{}
	mustResult(t, r.SetResultSink(s))
	mustResult(t, r.beginRun())
	l := &IntList{values: []int64{1, 2}}
	_, err := r.FunctionBySignature("fn lmcode_return_put(string, IntList) -> void").Fn([]Value{String("saved"), Object("IntList", l)})
	mustResult(t, err)
	l.values[0] = 99
	l.values = append(l.values, 3)
	r.endRun()
	if s.JSON() != `{"saved":[1,2]}` {
		t.Fatal("source mutation or arena teardown changed sink", s.JSON())
	}
	mustResult(t, r.beginRun())
	_, err = r.FunctionBySignature("fn lmcode_return_put(string, bool) -> void").Fn([]Value{String("entry"), Bool(true)})
	mustResult(t, err)
	r.endRun()
	if s.JSON() != `{"saved":[1,2],"entry":true}` {
		t.Fatal("reset between regions", s.JSON())
	}
	j, err := ParseJSON(`{"nested":["owned",true,null]}`)
	mustResult(t, err)
	mustResult(t, s.Set(Object("Json", j)))
	j.object[0].value.array = nil
	if s.JSON() != `{"nested":["owned",true,null]}` {
		t.Fatal("borrowed JSON", s.JSON())
	}
	s.Reset()
	if s.JSON() != "null" {
		t.Fatal("reset")
	}
}
func TestResultFrameNumericAndStrings(t *testing.T) {
	s := &ResultSink{}
	a, b := 0.1, 0.2
	mustResult(t, s.Set(Float(a+b)))
	r, _ := InvocationResultsFromManifest([]string{"test"}, nil)
	r.slots[0].sink = s
	mustResult(t, r.Complete(0))
	var frame strings.Builder
	if r.WriteFrame(&frame) != 0 {
		t.Fatal("frame failure")
	}
	if frame.String() != "LMCODE_RETURN_BEGIN\n{\"ok\":true,\"result\":0.30000000000000004}\nLMCODE_RETURN_END\n" {
		t.Fatal("shortest round trip", frame.String())
	}
	s = &ResultSink{}
	for _, c := range []struct {
		v    float64
		want string
	}{{math.Copysign(0, -1), "0"}, {math.SmallestNonzeroFloat64, "5e-324"}, {0.1, "0.1"}} {
		mustResult(t, s.Set(Float(c.v)))
		if s.JSON() != c.want {
			t.Fatal(s.JSON(), c.want)
		}
	}
	text := "雪\u2028<>&\x00\nLMCODE_RETURN_END"
	mustResult(t, s.Set(String(text)))
	var got string
	if err := json.Unmarshal([]byte(s.JSON()), &got); err != nil || got != text {
		t.Fatal(s.JSON(), err)
	}
	if strings.Contains(s.JSON(), "\\u003c") || strings.Contains(s.JSON(), "\\u2028") {
		t.Fatal("noncanonical string escaping", s.JSON())
	}
}
func TestResultAtomicErrorsAndOrder(t *testing.T) {
	s := &ResultSink{}
	mustResult(t, s.Put("a", Int(1)))
	mustResult(t, s.Put("b", Bool(false)))
	mustResult(t, s.Put("a", String("last")))
	mustResult(t, s.PutNull("nil"))
	if s.JSON() != `{"a":"last","b":false,"nil":null}` {
		t.Fatal(s.JSON())
	}
	before := s.JSON()
	for _, c := range []struct {
		v    Value
		code string
	}{{Float(math.Inf(1)), "nonfinite"}, {Float(math.NaN()), "nonfinite"}, {Int(9007199254740992), "unsafe_integer"}, {Float(9007199254740992), "unsafe_integer"}, {String("\xff"), "utf8"}, {Object("Widget", new(int)), "handle"}, {Object("Json", (*JSON)(nil)), "handle"}} {
		resultError(t, s.Put("bad/~", c.v), "lmcode return: invalid JSON value at /bad~1~0: "+c.code)
		if s.JSON() != before {
			t.Fatal("failed write changed result")
		}
	}
	mustResult(t, s.Set(Int(5)))
	resultError(t, s.Put("x", Int(1)), "lmcode return: field assignment requires object or null")
	if s.JSON() != "5" {
		t.Fatal("scalar replaced")
	}
	s.Reset()
	resultError(t, s.PutNull("\xff"), "lmcode return: invalid JSON value at : utf8")
	if s.JSON() != "null" {
		t.Fatal("invalid key promoted root")
	}
}
func TestResultRawNumbers(t *testing.T) {
	for _, raw := range []string{"9007199254740991", "-9007199254740991", "9007199254740991.0", "90071992547409910e-1", "9.007199254740991e15", "1.0000000000000000", "0e-999999999999999999999999999", "-0.0e99999999999999999999999999", "1e-20", "0.00001e20"} {
		if e := resultRawNumber(raw, ""); e != nil {
			t.Fatalf("safe raw %s: %v", raw, e)
		}
	}
	for _, raw := range []string{"9007199254740992", "9007199254740993", "-9007199254740993", "90071992547409930000e-4", "9.007199254740992e15", "1e20", "1e99999999999999999999999999"} {
		resultError(t, resultRawNumber(raw, ""), "lmcode return: invalid JSON value at : unsafe_integer")
	}
	s := &ResultSink{}
	for _, raw := range []string{"9007199254740992", "9007199254740993", "9.007199254740992e15", "9007199254740991.9"} {
		j, e := ParseJSON(raw)
		mustResult(t, e)
		resultError(t, s.Set(Object("Json", j)), "lmcode return: invalid JSON value at : unsafe_integer")
	}
	for _, raw := range []string{"1e-999", "0.00001e-999"} {
		j, e := ParseJSON(raw)
		mustResult(t, e)
		resultError(t, s.Set(Object("Json", j)), "lmcode return: invalid JSON value at : underflow")
	}
	j, e := ParseJSON(`{"a/b~c":[9007199254740993]}`)
	mustResult(t, e)
	resultError(t, s.Set(Object("Json", j)), "lmcode return: invalid JSON value at /a~1b~0c/0: unsafe_integer")
	j = &JSON{kind: jsonFloat, floating: 1, raw: "1e20"}
	resultError(t, s.Set(Object("Json", j)), "lmcode return: invalid JSON value at : unsafe_integer")
}
func TestResultCyclesAndHandles(t *testing.T) {
	s := &ResultSink{}
	j := &JSON{kind: jsonArray}
	j.array = []*JSON{j}
	resultError(t, s.Set(Object("Json", j)), "lmcode return: invalid JSON value at /0: cycle")
	j = &JSON{kind: jsonObject, object: []jsonMember{{key: "x", value: &JSON{}}, {key: "x", value: &JSON{}}}}
	resultError(t, s.Set(Object("Json", j)), "lmcode return: invalid JSON value at /x: handle")
	resultError(t, s.Set(Object("StrList", new(int))), "lmcode return: invalid JSON value at : handle")
}
func TestResultMissingAndReboundSink(t *testing.T) {
	r := NewRegistry()
	fn := r.FunctionBySignature("fn lmcode_return_null() -> void")
	if fn == nil {
		t.Fatal("missing null callback")
	}
	_, err := fn.Fn(nil)
	resultError(t, err, "lmcode return: no result sink")
	a, b := &ResultSink{}, &ResultSink{}
	mustResult(t, r.SetResultSink(a))
	mustResult(t, a.Set(Int(7)))
	mustResult(t, r.SetResultSink(b))
	_, err = fn.Fn(nil)
	mustResult(t, err)
	if a.JSON() != "7" || b.JSON() != "null" {
		t.Fatal("sink binding")
	}
	mustResult(t, r.SetResultSink(nil))
	_, err = fn.Fn(nil)
	resultError(t, err, "lmcode return: no result sink")
}
func TestResultConcurrentIsolation(t *testing.T) {
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Go(func() {
			r := NewRegistry()
			s := &ResultSink{}
			if err := r.SetResultSink(s); err != nil {
				t.Error(err)
				return
			}
			if err := r.beginRun(); err != nil {
				t.Error(err)
				return
			}
			defer r.endRun()
			_, err := r.FunctionBySignature("fn lmcode_return_set(int) -> void").Fn([]Value{Int(int64(i))})
			if err != nil || s.JSON() != strconv.Itoa(i) {
				t.Errorf("isolated sink %d: %s %v", i, s.JSON(), err)
			}
		})
	}
	group.Wait()
}

type resultShortWriter struct{}

func (resultShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestResultWriterFailure(t *testing.T) {
	r := completedResult(t)
	if code := r.WriteFrame(resultShortWriter{}); code != 4 {
		t.Fatal(code)
	}
}

func TestResultRegistryCopySharesBinding(t *testing.T) {
	r := NewRegistry()
	copy := *r
	s := &ResultSink{}
	mustResult(t, copy.SetResultSink(s))
	_, err := r.FunctionBySignature("fn lmcode_return_set(int) -> void").Fn([]Value{Int(4)})
	mustResult(t, err)
	if s.JSON() != "4" {
		t.Fatal("copied registry lost sink binding")
	}
}

func TestResultDistinctSinks(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	x, y := &ResultSink{}, &ResultSink{}
	mustResult(t, a.SetResultSink(x))
	mustResult(t, b.SetResultSink(y))
	_, err := a.FunctionBySignature("fn lmcode_return_set(string) -> void").Fn([]Value{String("left")})
	mustResult(t, err)
	_, err = b.FunctionBySignature("fn lmcode_return_set(string) -> void").Fn([]Value{String("right")})
	mustResult(t, err)
	if x.JSON() != `"left"` || y.JSON() != `"right"` {
		t.Fatal("shared global sink", x.JSON(), y.JSON())
	}
}
func TestResultFailedSetIsAtomic(t *testing.T) {
	s := &ResultSink{}
	mustResult(t, s.Set(Int(7)))
	resultError(t, s.Set(Float(math.Inf(1))), "lmcode return: invalid JSON value at : nonfinite")
	if s.JSON() != "7" {
		t.Fatal("failed set replaced prior sink", s.JSON())
	}
}

type resultDeferredWriter struct{}

func (resultDeferredWriter) Write(p []byte) (int, error) { return len(p), nil }
func (resultDeferredWriter) Flush() error                { return io.ErrClosedPipe }
func TestResultFlushFailure(t *testing.T) {
	r := completedResult(t)
	if code := r.WriteFrame(resultDeferredWriter{}); code != 4 {
		t.Fatalf("buffered frame failure lost: %v", code)
	}
}
func TestResultBorrowedChild(t *testing.T) {
	j, err := ParseJSON(`{"child":[1,true,null]}`)
	mustResult(t, err)
	child, err := j.Get("child")
	mustResult(t, err)
	s := &ResultSink{}
	mustResult(t, s.Set(Object("Json", child)))
	j.object[0].value.array = nil
	if s.JSON() != `[1,true,null]` {
		t.Fatal("borrowed child changed", s.JSON())
	}
}

func TestResultContainerSnapshots(t *testing.T) {
	s := &ResultSink{}
	a := &StrList{values: []string{"one"}}
	b := &FloatList{values: []float64{0.5, math.Copysign(0, -1)}}
	c := &StrMap{keys: []string{"s"}, values: map[string]string{"s": "v"}, positions: map[string]int{"s": 0}, live: 1, fenwick: []int{0, 1}}
	d := &IntMap{}
	d.data.set("i", 8)
	e := &FloatMap{}
	e.data.set("f", 0.25)
	for _, v := range []struct {
		key, typ string
		obj      any
	}{{"a", "StrList", a}, {"b", "FloatList", b}, {"c", "StrMap", c}, {"d", "IntMap", d}, {"e", "FloatMap", e}} {
		mustResult(t, s.Put(v.key, Object(v.typ, v.obj)))
	}
	a.values = nil
	b.values = nil
	c.values["s"] = "changed"
	d.data.clear()
	e.data.clear()
	if s.JSON() != `{"a":["one"],"b":[0.5,0],"c":{"s":"v"},"d":{"i":8},"e":{"f":0.25}}` {
		t.Fatal("borrowed container storage", s.JSON())
	}
}

func completedResult(t *testing.T) *InvocationResults {
	t.Helper()
	r, e := InvocationResultsFromManifest([]string{"test"}, nil)
	mustResult(t, e)
	_, e = r.Select(0, NewRegistry(), ResultContract{})
	mustResult(t, e)
	mustResult(t, r.Complete(0))
	return r
}
