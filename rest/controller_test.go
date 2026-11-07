package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/iotest"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

func entry(annotation string, extra ...string) lmcode.Entry {
	e := lmcode.Entry{Name: annotation, Annotation: annotation, Iface: HandlerInterface, Params: []string{"RestExchange"}, Returns: "void"}
	if len(extra) > 0 {
		e.Iface = extra[0]
	}
	if len(extra) > 1 {
		e.Name = extra[1]
	}
	return e
}

func fake(t testing.TB, entries ...lmcode.Entry) *Controller {
	t.Helper()
	c, err := Register(lmcode.NewRegistry(), Options{Input: strings.NewReader(""), Output: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	c.bound, c.listing = true, func() []lmcode.Entry { return entries }
	return c
}

// ---- registration -------------------------------------------------------------------------------------------

func declaredSignatures(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(suiteDir, "host.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Functions map[string][]struct {
			Params  []string `json:"params"`
			Returns string   `json:"returns"`
		} `json:"functions"`
		Types map[string]struct {
			Methods map[string][]struct {
				Params  []string `json:"params"`
				Returns string   `json:"returns"`
			} `json:"methods"`
		} `json:"types"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	var out []string
	for name, ds := range schema.Functions {
		for _, d := range ds {
			out = append(out, "fn "+name+"("+strings.Join(d.Params, ", ")+") -> "+d.Returns)
		}
	}
	for typ, def := range schema.Types {
		for name, ds := range def.Methods {
			for _, d := range ds {
				out = append(out, "method "+typ+"."+name+"("+strings.Join(d.Params, ", ")+") -> "+d.Returns)
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestRegistrationAddsExactlyTheEighteenDeclaredSignatures(t *testing.T) {
	declared := declaredSignatures(t)
	if len(declared) != 18 {
		t.Fatalf("host.schema.json declares %d signatures, want 18", len(declared))
	}
	plain := lmcode.NewRegistry()
	before := plain.Signatures()
	if len(plain.StdSignatures()) != 206 {
		t.Fatalf("default catalog has %d std signatures, want 206", len(plain.StdSignatures()))
	}
	for _, s := range before {
		if strings.Contains(s, "rest_") || strings.Contains(s, "RestExchange") {
			t.Fatalf("default registry leaks %s", s)
		}
	}
	if _, err := Register(plain, Options{}); err != nil {
		t.Fatal(err)
	}
	var added []string
	for _, s := range plain.Signatures() {
		if strings.Contains(s, "rest_") || strings.Contains(s, "RestExchange") {
			added = append(added, s)
		}
	}
	if strings.Join(added, "\n") != strings.Join(declared, "\n") {
		t.Fatalf("registered:\n%s\ndeclared:\n%s", strings.Join(added, "\n"), strings.Join(declared, "\n"))
	}
	if len(plain.Signatures()) != len(before)+18 || len(plain.StdSignatures()) != 206 {
		t.Fatal("registration must add exactly 18 host signatures and leave the std catalog alone")
	}
	if got := len(lmcode.NewRegistry().Signatures()); got != len(before) {
		t.Fatalf("a fresh default registry changed: %d vs %d", got, len(before))
	}
	if _, err := Register(plain, Options{}); err == nil {
		t.Fatal("registering twice must fail")
	}
}

// ---- rest_serve validation ----------------------------------------------------------------------------------

func TestServeValidation(t *testing.T) {
	none := fake(t)
	wantHostError(t, none.serve(), "rest: no routes registered")

	missing := fake(t)
	missing.route("GET", "/a", "nope")
	wantHostError(t, missing.serve(), "rest: route GET /a references missing annotation @nope")

	wrong := fake(t, entry("a", "interface Rule(Node) -> void", "check"))
	wrong.route("GET", "/a", "a")
	wantHostError(t, wrong.serve(), "rest: entry check must implement RestHandler(RestExchange) -> void")

	dup := fake(t, entry("a"), entry("a"))
	dup.route("GET", "/a", "a")
	wantHostError(t, dup.serve(), "rest: duplicate annotation @a")

	unbound, _ := Register(lmcode.NewRegistry(), Options{})
	unbound.route("GET", "/a", "a")
	wantHostError(t, unbound.serve(), "rest: no program is bound (run the program through the controller)")

	for name, c := range map[string]*Controller{"none": none, "missing": missing, "wrong": wrong, "dup": dup, "unbound": unbound} {
		if c.serving {
			t.Errorf("%s: a failed rest_serve must not start serving", name)
		}
	}
}

func TestServeIgnoresEntriesOfOtherInterfaces(t *testing.T) {
	c := fake(t, entry("a"), entry("a", "interface Rule(Node) -> void", "other"), entry("b", "interface Rule(Node) -> void", "rule"))
	c.route("GET", "/a", "a")
	if err := c.serve(); err != nil {
		t.Fatal(err)
	}
}

func TestServeCheckOrder(t *testing.T) {
	c := fake(t)
	c.serving = true
	c.bound = false
	wantHostError(t, c.serve(), "rest: rest_serve already called")
	c.serving = false
	wantHostError(t, c.serve(), "rest: no program is bound")
	c.bound = true
	wantHostError(t, c.serve(), "rest: no routes registered")
}

func TestLifecycleRoutesFreezeAndServeIsOnce(t *testing.T) {
	c := fake(t, entry("a"))
	if err := c.route("GET", "/a", "a"); err != nil {
		t.Fatal(err)
	}
	wantHostError(t, c.route("GET", "/a", "a"), "rest: duplicate route GET /a")
	if err := c.serve(); err != nil {
		t.Fatal(err)
	}
	wantHostError(t, c.route("GET", "/b", "a"), "rest: routes are frozen once rest_serve has been called")
	wantHostError(t, c.serve(), "rest: rest_serve already called")
	wantHostError(t, c.table.add("BAD", "/x", "a"), "rest: invalid route BAD /x")
}

// ---- exchange rules -----------------------------------------------------------------------------------------

type exchangeCall struct {
	t *testing.T
	r *lmcode.Registry
	x *exchange
}

func newExchangeCall(t *testing.T) exchangeCall {
	r := lmcode.NewRegistry()
	c, err := Register(r, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return exchangeCall{t, r, &exchange{state: &c.state, status: 200, params: map[string]string{}, query: map[string]string{}}}
}

func (e exchangeCall) call(sig string, args ...lmcode.Value) (lmcode.Value, error) {
	e.t.Helper()
	m := e.r.MethodBySignature("method RestExchange." + sig)
	if m == nil {
		e.t.Fatalf("method %s is not registered", sig)
	}
	return m.Fn(e.x, args)
}

func TestExchangeStatusRange(t *testing.T) {
	e := newExchangeCall(t)
	for _, bad := range []int64{199, 600, 0, -1} {
		_, err := e.call("status(int) -> void", lmcode.Int(bad))
		wantHostError(t, err, "rest: status")
		wantHostError(t, err, "out of range 200..599")
	}
	if _, err := e.call("status(int) -> void", lmcode.Int(599)); err != nil || e.x.status != 599 {
		t.Fatal(err, e.x.status)
	}
	_, err := e.call("status(int) -> void", lmcode.Int(199))
	wantHostError(t, err, "rest: status 199 out of range 200..599")
}

func TestExchangeHeaderValidation(t *testing.T) {
	e := newExchangeCall(t)
	set := func(name, value string) error {
		_, err := e.call("set_header(string, string) -> void", lmcode.String(name), lmcode.String(value))
		return err
	}
	for _, name := range []string{"", "a b", "a:b", "a\tb", "a\rb", "a\nb", "a\x00b", "a\x7fb"} {
		wantHostError(t, set(name, "v"), "rest: invalid response header")
	}
	wantHostError(t, set("X", "a\r\nInjected: 1"), "rest: invalid response header")
	wantHostError(t, set("X", "a\nb"), "rest: invalid response header")
	for _, name := range []string{"Content-Length", "connection", "TRANSFER-ENCODING"} {
		wantHostError(t, set(name, "1"), "rest: response header "+name+" is managed by the server")
	}
	if err := set("X-\xa0", "\xa0v"); err != nil {
		t.Fatalf("0xA0 is an ordinary byte: %v", err)
	}
	set("X-A", "1")
	set("x-a", "2")
	got := e.x.headers
	if len(got) != 2 || got[1] != [2]string{"x-a", "2"} {
		t.Fatalf("same name replaces in place, case-insensitively: %v", got)
	}
	e.call("json(string) -> void", lmcode.String("{}"))
	e.call("json(string) -> void", lmcode.String("[]"))
	if e.x.body != "[]" || len(e.x.headers) != 3 || e.x.headers[2] != [2]string{"Content-Type", "application/json"} {
		t.Fatalf("json replaces the body and sets the type once: %v", e.x.headers)
	}
}

func TestExchangeCountersAndStringsAreSeparateMaps(t *testing.T) {
	e := newExchangeCall(t)
	k := lmcode.String("k")
	for want := int64(1); want <= 2; want++ {
		if v, _ := e.call("increment(string) -> int", k); v.Int != want {
			t.Fatalf("increment: %d, want %d", v.Int, want)
		}
	}
	if v, _ := e.call("get(string) -> string", k); v.Str != "" {
		t.Fatal("counters and strings are separate maps")
	}
	e.call("set(string, string) -> void", k, lmcode.String("v"))
	if v, _ := e.call("get(string) -> string", k); v.Str != "v" {
		t.Fatal(v)
	}
	if v, _ := e.call("increment(string) -> int", k); v.Int != 3 {
		t.Fatal("setting a string must not reset the counter")
	}
	e.x.state.counts["max"] = 1<<63 - 1
	if v, _ := e.call("increment(string) -> int", lmcode.String("max")); v.Int != -1<<63 {
		t.Fatalf("increment wraps as int64, got %d", v.Int)
	}
}

func TestExchangeRequestAccessors(t *testing.T) {
	e := newExchangeCall(t)
	e.x.request = Request{Method: "PUT", Body: "b\xff", Headers: [][2]string{{"X-A", "1"}, {"x-a", "2"}, {"Other", "z"}}}
	e.x.params["id"], e.x.query["q"], e.x.path = "7", "", "/p"
	str := func(sig string, args ...string) string {
		var vs []lmcode.Value
		for _, a := range args {
			vs = append(vs, lmcode.String(a))
		}
		v, err := e.call(sig, vs...)
		if err != nil {
			t.Fatal(err)
		}
		return v.Str
	}
	boolean := func(sig, arg string) bool { v, _ := e.call(sig, lmcode.String(arg)); return v.Bool }
	if str("method() -> string") != "PUT" || str("path() -> string") != "/p" || str("body() -> string") != "b\xff" {
		t.Fatal("simple accessors")
	}
	if str("param(string) -> string", "id") != "7" || str("param(string) -> string", "nope") != "" {
		t.Fatal("param")
	}
	if str("header(string) -> string", "X-A") != "1, 2" || str("header(string) -> string", "missing") != "" {
		t.Fatal("repeated headers join with ', '")
	}
	if !boolean("has_header(string) -> bool", "OTHER") || boolean("has_header(string) -> bool", "nope") {
		t.Fatal("has_header")
	}
	if !boolean("has_query(string) -> bool", "q") || boolean("has_query(string) -> bool", "r") || str("query(string) -> string", "q") != "" {
		t.Fatal("query")
	}
	m := e.r.MethodBySignature("method RestExchange.method() -> string")
	if _, err := m.Fn("not an exchange", nil); err == nil || err.Error() != "invalid exchange" {
		t.Fatalf("a foreign self is refused: %v", err)
	}
}

// ---- dispatch against the real suite program ----------------------------------------------------------------

func dispatchSuite(t *testing.T, onError func(string, string)) *Controller {
	t.Helper()
	c, program := newSuiteController(t, Options{Input: strings.NewReader(""), Output: &bytes.Buffer{}, OnHandlerError: onError})
	if err := c.Run(program, nil); err != nil { // the suite body calls rest_serve(); the empty input ends serving at once
		t.Fatal(err)
	}
	return c
}

func get(target string) Request { return Request{Method: "GET", Target: target} }

func TestDispatchPercentDecodingAndQuery(t *testing.T) {
	c := dispatchSuite(t, nil)
	body := func(r Request) string { resp := c.Dispatch(r); return resp.Body }
	if got := body(get("/hello/w%C3%B6rld")); got != "hello, w\xc3\xb6rld" {
		t.Fatal(got)
	}
	if got := body(get("/hello/a%2Fb")); got != "hello, a/b" {
		t.Fatal(got)
	}
	if got := body(get("/hello/bob?greeting=a+b&greeting=ignored")); got != "a b, bob" {
		t.Fatalf("first key wins, + is a space: %q", got)
	}
	if got := body(get("/hello/bob?&&greeting")); got != ", bob" {
		t.Fatalf("a key without '=' has an empty value and empty pairs are skipped: %q", got)
	}
	if got := body(get("/hello/bob?greeting=%FF%00")); got != "\xff\x00, bob" {
		t.Fatalf("decoded bytes need not be UTF-8: %q", got)
	}
	for _, target := range []string{"/hello/%zz", "/nope%zz", "/hello/x?a=%4", "/hello/x?%=1", "/hello/x?a=%zz&b=1", "/health/%"} {
		if r := c.Dispatch(get(target)); r.Status != 400 || r.Body != `{"error":"bad request"}` {
			t.Errorf("%s: %d %q (invalid escapes are 400 before routing)", target, r.Status, r.Body)
		}
	}
	if r := c.Dispatch(get("/nope")); r.Status != 404 {
		t.Fatal(r)
	}
	if r := c.Dispatch(Request{Method: "BREW", Target: "/health"}); r.Status != 405 || r.Headers[1] != [2]string{"Allow", "GET, HEAD"} {
		t.Fatalf("%+v", r)
	}
}

func TestDispatchResponseShaping(t *testing.T) {
	c := dispatchSuite(t, nil)
	r := c.Dispatch(get("/health"))
	if r.Status != 200 || r.Headers[0] != [2]string{"Content-Type", "application/json"} || r.Body != `{"status":"ok"}` {
		t.Fatalf("%+v", r)
	}
	r = c.Dispatch(get("/hello/x"))
	if len(r.Headers) != 1 || r.Headers[0] != [2]string{"Content-Type", "text/plain; charset=utf-8"} {
		t.Fatalf("a non-empty body gets the default type: %+v", r)
	}
	r = c.Dispatch(Request{Method: "DELETE", Target: "/things/7"})
	if r.Status != 204 || r.Body != "" || len(r.Headers) != 1 || r.Headers[0] != [2]string{"X-Removed", "7"} {
		t.Fatalf("204 keeps headers and drops the body: %+v", r)
	}
	r = c.Dispatch(get("/files/"))
	if r.Status != 200 || r.Body != "" || len(r.Headers) != 0 {
		t.Fatalf("an empty body has no default type: %+v", r)
	}
	r = c.Dispatch(Request{Method: "HEAD", Target: "/health"})
	if r.Status != 200 || r.Body != `{"status":"ok"}` {
		t.Fatalf("HEAD runs the GET handler; the pump drops the body: %+v", r)
	}
}

func TestHandlerErrorsBecome500AndReachOnHandlerError(t *testing.T) {
	var entries, messages []string
	c := dispatchSuite(t, func(entry, message string) { entries, messages = append(entries, entry), append(messages, message) })
	r := c.Dispatch(get("/boom"))
	if r.Status != 500 || r.Body != `{"error":"internal error"}` {
		t.Fatalf("%+v", r)
	}
	if len(entries) != 1 || entries[0] != "boom" || !strings.Contains(strings.ToLower(messages[0]), "zero") {
		t.Fatalf("entry %v message %v", entries, messages)
	}
	if strings.Contains(r.Body, messages[0]) {
		t.Fatal("the VM message must never reach the client")
	}
	if r := c.Dispatch(get("/health")); r.Status != 200 {
		t.Fatal("the server keeps serving")
	}
	bad := c.Dispatch(Request{Method: "POST", Target: "/sum", Body: "not json"})
	if bad.Status != 500 || len(entries) != 2 || entries[1] != "sum" {
		t.Fatalf("%+v %v", bad, entries)
	}
}

// ---- the stream pump ----------------------------------------------------------------------------------------

func TestPumpReassemblesRequestsReadOneByteAtATime(t *testing.T) {
	input := []byte("POST /sum HTTP/1.1\r\nContent-Length: 21\r\n\r\n{\"values\":[10,20,12]}GET /count HTTP/1.1\r\n\r\n")
	var out bytes.Buffer
	c, program := newSuiteController(t, Options{Input: iotest.OneByteReader(bytes.NewReader(input)), Output: &out})
	if err := c.Run(program, nil); err != nil {
		t.Fatal(err)
	}
	r := parseResponses(t, out.Bytes(), []string{"POST", "GET"})
	if len(r) != 2 || r[0].status != 201 || r[0].body != `{"sum":42}` || r[1].body != "1" {
		t.Fatalf("%+v", r)
	}
}

func TestPumpDoesNotReadAfterRestStop(t *testing.T) {
	first := "POST /shutdown HTTP/1.1\r\nContent-Length: 0\r\n\r\n"
	reads := 0
	reader := readerFunc(func(p []byte) (int, error) {
		reads++
		if reads > 1 {
			t.Error("input was read after rest_stop()")
			return 0, errors.New("read after stop")
		}
		return copy(p, first+"GET /health HTTP/1.1\r\n\r\n"), nil
	})
	var out bytes.Buffer
	c, program := newSuiteController(t, Options{Input: reader, Output: &out})
	if err := c.Run(program, nil); err != nil {
		t.Fatal(err)
	}
	if r := parseResponses(t, out.Bytes(), []string{"POST"}); len(r) != 1 || r[0].status != 202 || r[0].body != "bye" {
		t.Fatalf("%q", out.String())
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestPumpReportsStreamErrors(t *testing.T) {
	boom := errors.New("pipe broke")
	c, program := newSuiteController(t, Options{Input: iotest.ErrReader(boom), Output: &bytes.Buffer{}})
	if err := c.Run(program, nil); !errors.Is(err, boom) {
		t.Fatalf("read error: %v", err)
	}
	c, program = newSuiteController(t, Options{Input: strings.NewReader("GET /health HTTP/1.1\r\n\r\n"), Output: failingWriter{boom}})
	if err := c.Run(program, nil); !errors.Is(err, boom) {
		t.Fatalf("write error: %v", err)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunReturnsBodyErrorsWithoutServing(t *testing.T) {
	registry := lmcode.NewRegistry()
	c, _ := Register(registry, Options{Input: readerFunc(func([]byte) (int, error) { t.Error("must not read"); return 0, nil }), Output: &bytes.Buffer{}})
	bytecode, _ := os.ReadFile(filepath.Join(suiteDir, "app.dslb"))
	schema, _ := os.ReadFile(filepath.Join(suiteDir, "host.schema.json"))
	program, err := lmcode.Load(bytecode, schema, registry)
	if err != nil {
		t.Fatal(err)
	}
	c.serving = true // routes are frozen, so the body's first rest_route fails and nothing is served
	wantHostError(t, c.Run(program, nil), "rest: routes are frozen once rest_serve has been called")
}
