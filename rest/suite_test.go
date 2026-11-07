package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

// The shared conformance suite (conformance/rest-suite, pinned by rest-suite.lock): scenarios.json groups
// stream / fatal / silent, replayed through in-memory streams and through the real process stdin/stdout.

var suiteDir = filepath.Join("..", "conformance", "rest-suite")

type scenario struct {
	Name       string            `json:"name"`
	Method     string            `json:"method"`
	Target     string            `json:"target"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	BodyRepeat *struct {
		Text  string `json:"text"`
		Count int    `json:"count"`
	} `json:"body_repeat"`
	Raw         *string `json:"raw"`
	StopsServer bool    `json:"stops_server"`
	Unanswered  bool    `json:"unanswered"`
	Expect      struct {
		Status  int                `json:"status"`
		Headers map[string]*string `json:"headers"`
		Body    *string            `json:"body"`
	} `json:"expect"`
}

type suiteFile struct {
	Options struct {
		MaxBodyBytes int64 `json:"max_body_bytes"`
	} `json:"options"`
	Stream []scenario `json:"stream"`
	Fatal  []scenario `json:"fatal"`
	Silent []scenario `json:"silent"`
}

func loadSuite(t testing.TB) suiteFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(suiteDir, "scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s suiteFile
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func latin1(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		b = append(b, byte(r))
	}
	return string(b)
}

// requestBytes composes a scenario as README.md of the suite prescribes.
func requestBytes(s scenario) []byte {
	if s.Raw != nil {
		return []byte(latin1(*s.Raw))
	}
	body := s.Body
	if s.BodyRepeat != nil {
		body = strings.Repeat(s.BodyRepeat.Text, s.BodyRepeat.Count)
	}
	lines := []string{s.Method + " " + s.Target + " HTTP/1.1", "Host: localhost"}
	names := make([]string, 0, len(s.Headers))
	for n := range s.Headers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		lines = append(lines, n+": "+s.Headers[n])
	}
	if body != "" || s.Method == "POST" || s.Method == "PUT" || s.Method == "PATCH" {
		lines = append(lines, "Content-Length: "+strconv.Itoa(len(body)))
	}
	return []byte(strings.Join(lines, "\r\n") + "\r\n\r\n" + body)
}

type parsed struct {
	status  int
	headers map[string]string
	body    string
	length  int
}

// parseResponses reads every response in out; methods[i] is the method of request i (HEAD responses carry no body bytes).
func parseResponses(t testing.TB, out []byte, methods []string) []parsed {
	t.Helper()
	var all []parsed
	for pos := 0; pos < len(out); {
		end := bytes.Index(out[pos:], []byte("\r\n\r\n"))
		if end < 0 {
			t.Fatalf("incomplete response head: %q", out[pos:])
		}
		lines := strings.Split(string(out[pos:pos+end]), "\r\n")
		var status int
		if _, err := fmt.Sscanf(lines[0], "HTTP/1.1 %d ", &status); err != nil {
			t.Fatalf("bad status line %q", lines[0])
		}
		r := parsed{status: status, headers: map[string]string{}, length: -1}
		for _, l := range lines[1:] {
			i := strings.IndexByte(l, ':')
			r.headers[strings.ToLower(l[:i])] = strings.TrimSpace(l[i+1:])
		}
		r.length, _ = strconv.Atoi(r.headers["content-length"])
		start := pos + end + 4
		if len(all) < len(methods) && methods[len(all)] == "HEAD" {
			pos = start
		} else {
			if start+r.length > len(out) {
				t.Fatalf("response body shorter than Content-Length")
			}
			r.body = string(out[start : start+r.length])
			pos = start + r.length
		}
		all = append(all, r)
	}
	return all
}

func checkResponse(t *testing.T, label string, r parsed, s scenario) {
	t.Helper()
	if r.status != s.Expect.Status {
		t.Errorf("%s: status %d, want %d", label, r.status, s.Expect.Status)
	}
	for name, want := range s.Expect.Headers {
		got, present := r.headers[name]
		switch {
		case want == nil && present:
			t.Errorf("%s: header %s must be absent, got %q", label, name, got)
		case want != nil && got != *want:
			t.Errorf("%s: header %s = %q, want %q", label, name, got, *want)
		}
	}
	if _, present := r.headers["connection"]; present {
		t.Errorf("%s: there is no Connection header", label)
	}
	if r.length < 0 {
		t.Errorf("%s: Content-Length missing", label)
	}
	if s.Method != "HEAD" && r.length != len(r.body) {
		t.Errorf("%s: Content-Length %d != body %d", label, r.length, len(r.body))
	}
	if s.Expect.Body != nil && r.body != *s.Expect.Body {
		t.Errorf("%s: body %q, want %q", label, r.body, *s.Expect.Body)
	}
}

// replay runs the three groups, each on a fresh server (serve), and returns the number of checks.
func replay(t *testing.T, serve func(input []byte) []byte) int {
	t.Helper()
	suite := loadSuite(t)
	count := 0
	var answered []scenario
	var all [][]byte
	for _, s := range suite.Stream {
		all = append(all, requestBytes(s))
		if !s.Unanswered {
			answered = append(answered, s)
		}
	}
	methods := make([]string, len(answered))
	for i, s := range answered {
		methods[i] = s.Method
	}
	responses := parseResponses(t, serve(bytes.Join(all, nil)), methods)
	if len(responses) != len(answered) {
		t.Fatalf("stream: %d responses for %d answered requests (none may follow rest_stop())", len(responses), len(answered))
	}
	for i, s := range answered {
		checkResponse(t, fmt.Sprintf("%s (%s %s)", s.Name, s.Method, s.Target), responses[i], s)
		count++
	}
	good := []byte("GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n")
	for _, s := range suite.Fatal {
		got := parseResponses(t, serve(append(requestBytes(s), good...)), []string{s.Method})
		if len(got) != 1 {
			t.Fatalf("%s: %d responses, want one error response and then the end", s.Name, len(got))
		}
		checkResponse(t, s.Name, got[0], s)
		count++
	}
	for _, s := range suite.Silent {
		if out := serve(requestBytes(s)); len(out) != 0 {
			t.Errorf("%s: output %q, want none", s.Name, out)
		}
		count++
	}
	return count
}

func newSuiteController(t testing.TB, options Options) (*Controller, *lmcode.Program) {
	t.Helper()
	suite := loadSuite(t)
	registry := lmcode.NewRegistry()
	if options.MaxBodyBytes == 0 {
		options.MaxBodyBytes = suite.Options.MaxBodyBytes
	}
	if options.OnHandlerError == nil {
		options.OnHandlerError = func(string, string) {}
	}
	c, err := Register(registry, options)
	if err != nil {
		t.Fatal(err)
	}
	bytecode, err := os.ReadFile(filepath.Join(suiteDir, "app.dslb"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join(suiteDir, "host.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	program, err := lmcode.Load(bytecode, schema, registry)
	if err != nil {
		t.Fatal(err)
	}
	return c, program
}

func serveInMemory(t *testing.T, input []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	c, program := newSuiteController(t, Options{Input: bytes.NewReader(input), Output: &out})
	if err := c.Run(program, lmcode.NewContext()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.Bytes()
}

var (
	buildOnce sync.Once
	hostPath  string
	buildErr  error
)

// serveHost builds cmd/rest-serve once and returns its path.
func serveHost(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "rest-serve")
		if err != nil {
			buildErr = err
			return
		}
		hostPath = filepath.Join(dir, "rest-serve")
		if runtime.GOOS == "windows" {
			hostPath += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", hostPath, "github.com/lmctl-ai/lmcode-go/cmd/rest-serve")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return hostPath
}

func serveProcess(t *testing.T, input []byte) []byte {
	t.Helper()
	suite := loadSuite(t)
	cmd := exec.Command(serveHost(t), filepath.Join(suiteDir, "app.dslb"), filepath.Join(suiteDir, "host.schema.json"), "--max-body-bytes", strconv.FormatInt(suite.Options.MaxBodyBytes, 10))
	var out, errs bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(input), &out, &errs
	if err := cmd.Run(); err != nil {
		t.Fatalf("rest-serve: %v\n%s", err, errs.String())
	}
	return out.Bytes()
}

func TestSuiteReplayThroughInMemoryStreams(t *testing.T) {
	n := replay(t, func(in []byte) []byte { return serveInMemory(t, in) })
	if n != 39 {
		t.Fatalf("replayed %d checks, want 39 (32 answered stream requests + 4 fatal + 3 silent)", n)
	}
}

func TestSuiteReplayThroughProcessStdinStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns cmd/rest-serve")
	}
	n := replay(t, func(in []byte) []byte { return serveProcess(t, in) })
	if n != 39 {
		t.Fatalf("replayed %d checks, want 39", n)
	}
}

func TestServeHostExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns cmd/rest-serve")
	}
	host := serveHost(t)
	exit := func(args ...string) int {
		err := exec.Command(host, args...).Run()
		if err == nil {
			return 0
		}
		return err.(*exec.ExitError).ExitCode()
	}
	if code := exit(); code != 2 {
		t.Errorf("usage: exit %d", code)
	}
	if code := exit(filepath.Join(suiteDir, "missing.dslb"), filepath.Join(suiteDir, "host.schema.json")); code != 3 {
		t.Errorf("missing program: exit %d", code)
	}
	if code := exit(filepath.Join(suiteDir, "app.dslb"), filepath.Join(suiteDir, "scenarios.json")); code != 3 {
		t.Errorf("schema mismatch: exit %d", code)
	}
}
