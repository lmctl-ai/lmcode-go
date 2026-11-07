package rest

import (
	"fmt"
	"io"
	"os"
	"strings"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

// HandlerInterface is the interface an entry must implement to be routable.
const HandlerInterface = "interface RestHandler(RestExchange) -> void"

// DefaultMaxBodyBytes is the default request body limit (1 MiB).
const DefaultMaxBodyBytes = 1024 * 1024

// Options configure a Controller; the zero value serves os.Stdin to os.Stdout.
type Options struct {
	Input          io.Reader                   // default os.Stdin
	Output         io.Writer                   // default os.Stdout
	MaxBodyBytes   int64                       // default 1 MiB; a Content-Length above it is 413
	OnHandlerError func(entry, message string) // default: one line on stderr
}

type entryRef struct {
	index int
	name  string
}

type state struct {
	values map[string]string
	counts map[string]int64
}

// Controller owns the routes, the shared state and the streams of one rest server.
type Controller struct {
	registry *lmcode.Registry
	input    io.Reader
	output   io.Writer
	maxBody  int64
	onError  func(entry, message string)

	state   state
	table   routeTable
	program *lmcode.Program
	context *lmcode.Context
	bound   bool
	entries map[string]entryRef
	// listing returns the program's entries; call runs one handler. Both are replaced by unit tests.
	listing func() []lmcode.Entry
	call    func(index int, x *exchange) error

	serving bool
	stopped bool
}

// Run binds program, runs its body, then (if the body called rest_serve and not rest_stop) serves the input
// stream until EOF, rest_stop() or a framing error. It returns the body's error, or a stream error.
func (c *Controller) Run(program *lmcode.Program, ctx *lmcode.Context) error {
	if ctx == nil {
		ctx = lmcode.NewContext()
	}
	c.program, c.context, c.bound, c.stopped = program, ctx, true, false
	c.listing = program.Entries
	c.call = func(index int, x *exchange) error {
		_, err := program.Call(index, ctx, c.registry, []lmcode.Value{lmcode.Obj("RestExchange", x)})
		return err
	}
	if err := program.Run(ctx, c.registry); err != nil {
		return err
	}
	if c.serving && !c.stopped {
		return c.pump()
	}
	return nil
}

// Stop ends serving once the current response is written (what rest_stop() does).
func (c *Controller) Stop() { c.stopped = true }

// ---- lmcode-facing operations -----------------------------------------------------------------------------

func (c *Controller) route(method, path, annotation string) error {
	if c.serving {
		return hostErr("rest: routes are frozen once rest_serve has been called")
	}
	return c.table.add(method, path, annotation)
}

func (c *Controller) serve() error {
	if c.serving {
		return hostErr("rest: rest_serve already called")
	}
	if !c.bound {
		return hostErr("rest: no program is bound (run the program through the controller)")
	}
	if err := c.bind(); err != nil {
		return err
	}
	c.serving = true
	return nil
}

func (c *Controller) bind() error {
	if len(c.table.routes) == 0 {
		return hostErr("rest: no routes registered")
	}
	entries := c.listing()
	c.entries = map[string]entryRef{}
	for i, e := range entries {
		if e.Iface != HandlerInterface {
			continue
		}
		if _, dup := c.entries[e.Annotation]; dup {
			return hostErr("rest: duplicate annotation @%s", e.Annotation)
		}
		c.entries[e.Annotation] = entryRef{i, e.Name}
	}
	for _, r := range c.table.routes {
		if _, ok := c.entries[r.annotation]; ok {
			continue
		}
		for _, e := range entries {
			if e.Annotation == r.annotation {
				return hostErr("rest: entry %s must implement RestHandler(RestExchange) -> void", e.Name)
			}
		}
		return hostErr("rest: route %s %s references missing annotation @%s", r.method, r.path, r.annotation)
	}
	return nil
}

// ---- the transport-independent core -----------------------------------------------------------------------

type exchange struct {
	state   *state
	request Request
	params  map[string]string
	query   map[string]string
	path    string
	status  int
	headers [][2]string // insertion order; a same-name set replaces in place
	body    string
}

func (x *exchange) requestHeaders(name string) []string {
	wanted := asciiLower(name)
	var out []string
	for _, h := range x.request.Headers {
		if asciiLower(h[0]) == wanted {
			out = append(out, h[1])
		}
	}
	return out
}

func (x *exchange) setHeader(name, value string) {
	key := asciiLower(name)
	for i, h := range x.headers {
		if asciiLower(h[0]) == key {
			x.headers[i] = [2]string{name, value}
			return
		}
	}
	x.headers = append(x.headers, [2]string{name, value})
}

func (x *exchange) hasHeader(name string) bool {
	key := asciiLower(name)
	for _, h := range x.headers {
		if asciiLower(h[0]) == key {
			return true
		}
	}
	return false
}

// Dispatch is the transport-independent core: one request in, one response out. A handler error becomes a 500.
func (c *Controller) Dispatch(req Request) Response {
	rawPath, queryText := req.Target, ""
	if mark := strings.IndexByte(req.Target, '?'); mark >= 0 {
		rawPath, queryText = req.Target[:mark], req.Target[mark+1:]
	}
	path, ok := percentDecode(rawPath, false)
	if !ok {
		return errorResponse(400)
	}
	query := map[string]string{}
	if queryText != "" {
		for _, pair := range strings.Split(queryText, "&") {
			if pair == "" {
				continue
			}
			k, v := pair, ""
			if eq := strings.IndexByte(pair, '='); eq >= 0 {
				k, v = pair[:eq], pair[eq+1:]
			}
			key, ok1 := percentDecode(k, true)
			value, ok2 := percentDecode(v, true)
			if !ok1 || !ok2 {
				return errorResponse(400)
			}
			if _, seen := query[key]; !seen {
				query[key] = value
			}
		}
	}
	m := c.table.match(req.Method, rawPath)
	switch m.status {
	case 404:
		return errorResponse(404)
	case 405:
		return errorResponse(405, [2]string{"Allow", m.allow})
	}
	target, found := c.entries[m.route.annotation]
	if !found {
		c.onError("", "rest: dispatch before rest_serve bound the routes")
		return errorResponse(500)
	}
	x := &exchange{state: &c.state, request: req, params: m.params, query: query, path: path, status: 200}
	if err := c.invoke(target, x); err != nil {
		c.onError(target.name, err.Error())
		return errorResponse(500)
	}
	headers := append([][2]string(nil), x.headers...)
	body := x.body
	if x.status == 204 || x.status == 304 {
		body = ""
	} else if body != "" && !x.hasHeader("content-type") {
		headers = append([][2]string{{"Content-Type", "text/plain; charset=utf-8"}}, headers...)
	}
	return Response{Status: x.status, Headers: headers, Body: body}
}

func (c *Controller) invoke(target entryRef, x *exchange) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return c.call(target.index, x)
}

// pump reads framed requests and writes one response each until EOF, rest_stop() or a framing error.
func (c *Controller) pump() error {
	var buf []byte
	chunk := make([]byte, 32*1024)
	var readErr error
	for {
		t := takeRequest(buf, c.maxBody)
		if t.fatal != 0 {
			_, err := c.output.Write(encodeResponse(errorResponse(t.fatal), false))
			return err
		}
		if t.need {
			if readErr != nil {
				if readErr == io.EOF {
					return nil // a request truncated by EOF is dropped without a response
				}
				return readErr
			}
			n, err := c.input.Read(chunk)
			buf = append(buf, chunk[:n]...)
			readErr = err
			continue
		}
		buf = append([]byte(nil), buf[t.consumed:]...)
		if _, err := c.output.Write(encodeResponse(c.Dispatch(t.request), t.request.Method == "HEAD")); err != nil {
			return err
		}
		if c.stopped {
			return nil
		}
	}
}

// ---- registration ----------------------------------------------------------------------------------------

// Register adds the 18 rest signatures to registry and returns the controller that owns the server.
// The default registry never calls it: the module is opt-in.
func Register(registry *lmcode.Registry, options Options) (*Controller, error) {
	c := &Controller{registry: registry, input: options.Input, output: options.Output, maxBody: options.MaxBodyBytes, onError: options.OnHandlerError}
	c.state = state{values: map[string]string{}, counts: map[string]int64{}}
	if c.input == nil {
		c.input = os.Stdin
	}
	if c.output == nil {
		c.output = os.Stdout
	}
	if c.maxBody <= 0 {
		c.maxBody = DefaultMaxBodyBytes
	}
	if c.onError == nil {
		c.onError = func(entry, message string) { fmt.Fprintf(os.Stderr, "rest: handler %s failed: %s\n", entry, message) }
	}
	var first error
	fn := func(name string, params []string, returns string, f lmcode.Function) {
		if err := registry.Function(name, params, returns, f); err != nil && first == nil {
			first = err
		}
	}
	method := func(name string, params []string, returns string, f func(x *exchange, a []lmcode.Value) (lmcode.Value, error)) {
		err := registry.Method("RestExchange", name, params, returns, func(self any, a []lmcode.Value) (lmcode.Value, error) {
			x, ok := self.(*exchange)
			if !ok || x == nil {
				return lmcode.None(), lmcode.NewHostError("invalid exchange")
			}
			return f(x, a)
		})
		if err != nil && first == nil {
			first = err
		}
	}
	none := lmcode.None()
	S := []string{"string"}

	fn("rest_route", []string{"string", "string", "string"}, "void", func(a []lmcode.Value) (lmcode.Value, error) {
		return none, c.route(a[0].Str, a[1].Str, a[2].Str)
	})
	fn("rest_serve", nil, "void", func(a []lmcode.Value) (lmcode.Value, error) { return none, c.serve() })
	fn("rest_stop", nil, "void", func(a []lmcode.Value) (lmcode.Value, error) { c.Stop(); return none, nil })

	method("method", nil, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(x.request.Method), nil
	})
	method("path", nil, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(x.path), nil
	})
	method("query", S, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(x.query[a[0].Str]), nil
	})
	method("has_query", S, "bool", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		_, ok := x.query[a[0].Str]
		return lmcode.Bool(ok), nil
	})
	method("param", S, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(x.params[a[0].Str]), nil
	})
	method("header", S, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(strings.Join(x.requestHeaders(a[0].Str), ", ")), nil
	})
	method("has_header", S, "bool", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Bool(len(x.requestHeaders(a[0].Str)) > 0), nil
	})
	method("body", nil, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(x.request.Body), nil
	})
	method("status", []string{"int"}, "void", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		if code := a[0].Int; code < 200 || code > 599 {
			return none, hostErr("rest: status %d out of range 200..599", code)
		}
		x.status = int(a[0].Int)
		return none, nil
	})
	method("set_header", []string{"string", "string"}, "void", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		name, value := a[0].Str, a[1].Str
		bad := name == "" || strings.ContainsAny(value, "\r\n")
		for i := 0; i < len(name) && !bad; i++ {
			bad = isSpace(name[i]) || name[i] == ':'
		}
		if bad {
			return none, hostErr("rest: invalid response header")
		}
		switch asciiLower(name) {
		case "content-length", "connection", "transfer-encoding":
			return none, hostErr("rest: response header %s is managed by the server", name)
		}
		x.setHeader(name, value)
		return none, nil
	})
	method("send", S, "void", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		x.body = a[0].Str
		return none, nil
	})
	method("json", S, "void", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		x.body = a[0].Str
		x.setHeader("Content-Type", "application/json")
		return none, nil
	})
	method("get", S, "string", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(x.state.values[a[0].Str]), nil
	})
	method("set", []string{"string", "string"}, "void", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		x.state.values[a[0].Str] = a[1].Str
		return none, nil
	})
	method("increment", S, "int", func(x *exchange, a []lmcode.Value) (lmcode.Value, error) {
		x.state.counts[a[0].Str]++
		return lmcode.Int(x.state.counts[a[0].Str]), nil
	})
	if first != nil {
		return nil, first
	}
	return c, nil
}
