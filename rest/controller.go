package rest

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

// HandlerInterface is the interface an entry must implement to be routable.
const (
	HandlerInterface     = "interface RestHandler(RestExchange) -> void"
	RestGetInterface     = "interface RestGet(RestExchange) -> void"
	RestPostInterface    = "interface RestPost(RestExchange) -> void"
	RestPutInterface     = "interface RestPut(RestExchange) -> void"
	RestDeleteInterface  = "interface RestDelete(RestExchange) -> void"
	RestPatchInterface   = "interface RestPatch(RestExchange) -> void"
	RestHeadInterface    = "interface RestHead(RestExchange) -> void"
	RestOptionsInterface = "interface RestOptions(RestExchange) -> void"
	CliCommandInterface  = "interface CliCommand(RestExchange) -> void"
	CliOptionInterface   = "interface CliOption(RestExchange) -> void"
)

func methodForRestInterface(iface string) string {
	switch iface {
	case RestGetInterface:
		return "GET"
	case RestPostInterface:
		return "POST"
	case RestPutInterface:
		return "PUT"
	case RestDeleteInterface:
		return "DELETE"
	case RestPatchInterface:
		return "PATCH"
	case RestHeadInterface:
		return "HEAD"
	case RestOptionsInterface:
		return "OPTIONS"
	default:
		return ""
	}
}

func isRestOrCliInterface(iface string) bool {
	return iface == HandlerInterface || iface == CliCommandInterface || iface == CliOptionInterface || methodForRestInterface(iface) != ""
}

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

	state                  state
	configPrefix           string
	table                  routeTable
	program                *lmcode.Program
	context                *lmcode.Context
	bound                  bool
	inControllerRun        bool
	autoRoutesMaterialized bool
	autoEntryIndex         map[string]entryRef
	autoEntryIndices       map[int]bool
	entries                map[string]entryRef
	cliOptions             map[string]entryRef
	// listing returns the program's entries; call runs one handler. Both are replaced by unit tests.
	listing func() []lmcode.Entry
	call    func(index int, x *exchange) error

	serving bool
	stopped bool
}

func (c *Controller) resetBindings(listing func() []lmcode.Entry, call func(index int, x *exchange) error) {
	c.bound = true
	c.listing = listing
	c.call = call
	c.autoRoutesMaterialized = false
	c.autoEntryIndex = map[string]entryRef{}
	c.autoEntryIndices = map[int]bool{}
	c.entries = map[string]entryRef{}
	c.cliOptions = map[string]entryRef{}
	c.configPrefix = ""
}

// Run binds program, runs its body, then (if the body called rest_serve and not rest_stop) serves the input
// stream until EOF, rest_stop() or a framing error. It returns the body's error, or a stream error.
func (c *Controller) Run(program *lmcode.Program, ctx *lmcode.Context) error {
	if ctx == nil {
		ctx = lmcode.NewContext()
	}
	c.inControllerRun = true
	defer func() { c.inControllerRun = false }()
	c.program, c.context, c.stopped = program, ctx, false
	c.resetBindings(program.Entries, func(index int, x *exchange) error {
		saved, wasActive := c.registry.SuspendStdRun()
		defer c.registry.ResumeStdRun(saved, wasActive)
		_, err := program.Call(index, ctx, c.registry, []lmcode.Value{lmcode.Obj("RestExchange", x)})
		return err
	})
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

func (c *Controller) config(key, value string) error {
	if c.serving {
		return hostErr("rest: routes are frozen once rest_serve has been called")
	}
	c.state.values[key] = value
	if key == "prefix" || key == "base_path" {
		c.configPrefix = value
	} else if key == "max_body_bytes" && value != "" {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n > 0 {
			c.maxBody = n
		}
	}
	return nil
}

func (c *Controller) normalizedPrefix() string {
	if c.configPrefix == "" {
		return ""
	}
	p := c.configPrefix
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	if p == "/" {
		return ""
	}
	return p
}

func (c *Controller) materializeAutoRoutes() error {
	if c.autoRoutesMaterialized {
		return nil
	}
	c.autoRoutesMaterialized = true
	if c.listing == nil {
		return nil
	}
	if c.autoEntryIndex == nil {
		c.autoEntryIndex = map[string]entryRef{}
	}
	if c.autoEntryIndices == nil {
		c.autoEntryIndices = map[int]bool{}
	}
	if c.cliOptions == nil {
		c.cliOptions = map[string]entryRef{}
	}
	prefix := c.normalizedPrefix()
	entries := c.listing()
	for i, e := range entries {
		ann := e.Annotation
		if e.Iface == CliOptionInterface {
			c.cliOptions[ann] = entryRef{i, e.Name}
			stripped := strings.TrimLeft(ann, "-")
			if stripped != "" {
				c.cliOptions[stripped] = entryRef{i, e.Name}
			}
			c.autoEntryIndices[i] = true
			continue
		}
		var method, path string
		if m := methodForRestInterface(e.Iface); m != "" {
			if sp := strings.IndexByte(ann, ' '); sp >= 0 {
				method, path = ann[:sp], ann[sp+1:]
			} else if strings.HasPrefix(ann, "/") {
				method, path = m, ann
			} else {
				continue
			}
		} else if e.Iface == HandlerInterface {
			if sp := strings.IndexByte(ann, ' '); sp >= 0 {
				method, path = ann[:sp], ann[sp+1:]
			} else if strings.HasPrefix(ann, "/") {
				method, path = "GET", ann
			} else {
				continue
			}
		} else if e.Iface == CliCommandInterface {
			if sp := strings.IndexByte(ann, ' '); sp >= 0 {
				method, path = ann[:sp], ann[sp+1:]
			} else {
				method, path = "GET", ann
			}
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
		} else {
			continue
		}
		if prefix != "" && !strings.HasPrefix(path, prefix) {
			if path == "/" {
				path = prefix
			} else {
				path = prefix + path
			}
		}
		tag := fmt.Sprintf("__auto_entry_%d", i)
		if err := c.table.add(method, path, tag); err != nil {
			return err
		}
		c.autoEntryIndex[tag] = entryRef{i, e.Name}
		c.autoEntryIndices[i] = true
	}
	return nil
}

func (c *Controller) ensureDispatchReady() error {
	if err := c.materializeAutoRoutes(); err != nil {
		return err
	}
	if len(c.entries) == 0 && len(c.autoEntryIndex) > 0 {
		c.entries = map[string]entryRef{}
		for k, v := range c.autoEntryIndex {
			c.entries[k] = v
		}
		if c.listing != nil {
			for i, e := range c.listing() {
				if !isRestOrCliInterface(e.Iface) || e.Iface == CliOptionInterface || c.autoEntryIndices[i] {
					continue
				}
				if _, exists := c.entries[e.Annotation]; !exists {
					c.entries[e.Annotation] = entryRef{i, e.Name}
				}
			}
		}
	}
	return nil
}

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
	if err := c.materializeAutoRoutes(); err != nil {
		return err
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
	for k, v := range c.autoEntryIndex {
		c.entries[k] = v
	}
	for i, e := range entries {
		if !isRestOrCliInterface(e.Iface) || e.Iface == CliOptionInterface {
			continue
		}
		if c.autoEntryIndices != nil && c.autoEntryIndices[i] {
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

func (c *Controller) cli(argv []string) (string, error) {
	if !c.bound || c.call == nil {
		return "", hostErr("rest: no program is bound (run the program through the controller)")
	}
	if err := c.ensureDispatchReady(); err != nil {
		return "", err
	}
	method := "GET"
	body := ""
	methodExplicit := false
	var positionals []string
	type optVal struct{ flag, key, val string }
	var parsedOpts []optVal

	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			if body == "" && i+1 < len(argv) {
				body = strings.Join(argv[i+1:], " ")
			}
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			flag, key, val := arg, "", "true"
			if eq := strings.IndexByte(arg, '='); eq >= 0 {
				flag, val = arg[:eq], arg[eq+1:]
				key = strings.TrimLeft(flag, "-")
			} else {
				key = strings.TrimLeft(flag, "-")
				if i+1 < len(argv) && (argv[i+1] == "" || argv[i+1][0] != '-') {
					next := argv[i+1]
					isSpecial := key == "method" || key == "X" || key == "body" || key == "d"
					probePath := next
					if !strings.HasPrefix(probePath, "/") {
						probePath = "/" + probePath
					}
					nextIsRootCmd := len(positionals) == 0 && !isSpecial && c.table.match("GET", probePath).status != 404
					if !nextIsRootCmd {
						i++
						val = argv[i]
					}
				}
			}
			if key == "method" || key == "X" {
				method = strings.ToUpper(val)
				methodExplicit = true
			} else if key == "body" || key == "d" {
				body = val
			}
			parsedOpts = append(parsedOpts, optVal{flag, key, val})
		} else {
			positionals = append(positionals, arg)
		}
	}

	path := "/"
	if len(positionals) > 0 {
		var joined string
		for i, seg := range positionals {
			if i == 0 && strings.HasPrefix(seg, "/") {
				joined = seg
			} else {
				joined += "/" + seg
			}
		}
		if joined != "" {
			path = joined
		}
	}
	prefix := c.normalizedPrefix()
	if prefix != "" && !strings.HasPrefix(path, prefix) {
		if path == "/" {
			path = prefix
		} else {
			path = prefix + path
		}
	}

	query := map[string]string{}
	for _, opt := range parsedOpts {
		query[opt.key] = opt.val
		query[opt.flag] = opt.val
		c.state.values[opt.key] = opt.val
	}
	x := &exchange{
		state:   &c.state,
		request: Request{Method: method, Target: path, Body: body},
		params:  map[string]string{},
		query:   query,
		path:    path,
		status:  200,
	}
	for _, opt := range parsedOpts {
		target, ok := c.cliOptions[opt.flag]
		if !ok {
			target, ok = c.cliOptions[opt.key]
		}
		if ok {
			if err := c.invoke(target, x); err != nil {
				return "", err
			}
		}
	}
	if c.stopped {
		return c.responseOf(x).Body, nil
	}

	m := c.table.match(method, path)
	if m.status == 405 && !methodExplicit {
		if body != "" && c.table.match("POST", path).status == 0 {
			x.request.Method = "POST"
			m = c.table.match("POST", path)
		} else {
			for _, cand := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"} {
				probe := c.table.match(cand, path)
				if probe.status == 0 {
					x.request.Method = cand
					m = probe
					break
				}
			}
		}
	}
	switch m.status {
	case 404:
		if len(positionals) == 0 && x.body != "" {
			return c.responseOf(x).Body, nil
		}
		return errorResponse(404).Body, nil
	case 405:
		return errorResponse(405, [2]string{"Allow", m.allow}).Body, nil
	}
	target, found := c.entries[m.route.annotation]
	if !found {
		return errorResponse(500).Body, nil
	}
	x.params = m.params
	if err := c.invoke(target, x); err != nil {
		c.onError(target.name, err.Error())
		return errorResponse(500).Body, nil
	}
	return c.responseOf(x).Body, nil
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

func (c *Controller) responseOf(x *exchange) Response {
	headers := append([][2]string(nil), x.headers...)
	body := x.body
	if x.status == 204 || x.status == 304 {
		body = ""
	} else if body != "" && !x.hasHeader("content-type") {
		headers = append([][2]string{{"Content-Type", "text/plain; charset=utf-8"}}, headers...)
	}
	return Response{Status: x.status, Headers: headers, Body: body}
}

// Dispatch is the transport-independent core: one request in, one response out. A handler error becomes a 500.
func (c *Controller) Dispatch(req Request) Response {
	_ = c.ensureDispatchReady()
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
	return c.responseOf(x)
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

// Register adds or overrides the 20 rest signatures in registry and returns the controller that owns the server.
func Register(registry *lmcode.Registry, options Options) (*Controller, error) {
	if err := registry.MarkRestRegistered(); err != nil {
		return nil, err
	}
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
	registry.SetRestHooks(
		func(program *lmcode.Program, ctx *lmcode.Context, reg *lmcode.Registry) (bool, error) {
			if c.inControllerRun {
				return false, nil
			}
			c.program, c.context, c.serving, c.stopped = program, ctx, false, false
			c.table = routeTable{}
			c.resetBindings(program.Entries, func(index int, x *exchange) error {
				saved, wasActive := reg.SuspendStdRun()
				defer reg.ResumeStdRun(saved, wasActive)
				_, err := program.Call(index, ctx, reg, []lmcode.Value{lmcode.Obj("RestExchange", x)})
				return err
			})
			return true, nil
		},
		func(program *lmcode.Program, ctx *lmcode.Context, reg *lmcode.Registry) error {
			if c.serving && !c.stopped {
				return c.pump()
			}
			return nil
		},
	)
	var first error
	fn := func(name string, params []string, returns string, f lmcode.Function, dry lmcode.Function) {
		if err := registry.OverrideStdFunction(name, params, returns, f, lmcode.Dry(dry)); err != nil && first == nil {
			first = err
		}
	}
	method := func(name string, params []string, returns string, f func(x *exchange, a []lmcode.Value) (lmcode.Value, error)) {
		cb := lmcode.Method(func(self any, a []lmcode.Value) (lmcode.Value, error) {
			x, ok := self.(*exchange)
			if !ok || x == nil {
				return lmcode.None(), lmcode.NewHostError("invalid exchange")
			}
			return f(x, a)
		})
		err := registry.OverrideStdMethod("RestExchange", name, params, returns, cb, lmcode.Dry(cb))
		if err != nil && first == nil {
			first = err
		}
	}
	none := lmcode.None()
	dryNone := func([]lmcode.Value) (lmcode.Value, error) { return none, nil }
	S := []string{"string"}

	fn("rest_config", []string{"string", "string"}, "void", func(a []lmcode.Value) (lmcode.Value, error) {
		return none, c.config(a[0].Str, a[1].Str)
	}, dryNone)
	fn("rest_route", []string{"string", "string", "string"}, "void", func(a []lmcode.Value) (lmcode.Value, error) {
		return none, c.route(a[0].Str, a[1].Str, a[2].Str)
	}, dryNone)
	fn("rest_serve", nil, "void", func(a []lmcode.Value) (lmcode.Value, error) { return none, c.serve() }, dryNone)
	fn("rest_stop", nil, "void", func(a []lmcode.Value) (lmcode.Value, error) { c.Stop(); return none, nil }, dryNone)
	fn("rest_cli", []string{"StrList"}, "string", func(a []lmcode.Value) (lmcode.Value, error) {
		list, ok := a[0].Obj.(*lmcode.StrList)
		if !ok || list == nil {
			return lmcode.String(""), lmcode.NewHostError("rest_cli: invalid StrList")
		}
		out, err := c.cli(list.Values())
		return lmcode.String(out), err
	}, func([]lmcode.Value) (lmcode.Value, error) { return lmcode.String(""), nil })

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
