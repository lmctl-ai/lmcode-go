package lmcode

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type stdRestSegKind int

const (
	stdRestLit stdRestSegKind = iota
	stdRestParam
	stdRestRest
)

type stdRestSeg struct {
	kind  stdRestSegKind
	value string
}

type stdRestRoute struct {
	method     string
	path       string
	segments   []stdRestSeg
	shape      string
	annotation string
}

type stdRestTable struct {
	routes []*stdRestRoute
}

var stdRestMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

func stdRestValidName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		alpha := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
		if !(alpha || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func stdRestPercentDecode(text string, plus bool) (string, bool) {
	if !strings.ContainsAny(text, "%+") {
		return text, true
	}
	out := make([]byte, 0, len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '%' {
			if i+2 >= len(text) {
				return "", false
			}
			h1, h2 := text[i+1], text[i+2]
			unhex := func(b byte) (byte, bool) {
				switch {
				case b >= '0' && b <= '9':
					return b - '0', true
				case b >= 'a' && b <= 'f':
					return b - 'a' + 10, true
				case b >= 'A' && b <= 'F':
					return b - 'A' + 10, true
				default:
					return 0, false
				}
			}
			v1, ok1 := unhex(h1)
			v2, ok2 := unhex(h2)
			if !ok1 || !ok2 {
				return "", false
			}
			out = append(out, v1<<4|v2)
			i += 2
		} else if plus && c == '+' {
			out = append(out, ' ')
		} else {
			out = append(out, c)
		}
	}
	return string(out), true
}

func stdRestParseRoute(method, path string) (*stdRestRoute, error) {
	invalid := func(reason string) error {
		return NewHostError(fmt.Sprintf("rest: invalid route %s %s: %s", method, path, reason))
	}
	known := false
	for _, m := range stdRestMethods {
		known = known || m == method
	}
	if !known {
		return nil, invalid("method must be one of " + strings.Join(stdRestMethods, ", "))
	}
	if !strings.HasPrefix(path, "/") {
		return nil, invalid("path must start with '/'")
	}
	for i := 0; i < len(path); i++ {
		if path[i] <= 0x20 || path[i] == 0x7f || path[i] == '?' || path[i] == '#' {
			return nil, invalid("path must not contain whitespace, control characters, ? or #")
		}
	}
	raw := strings.Split(path[1:], "/")
	names := map[string]bool{}
	segs := make([]stdRestSeg, 0, len(raw))
	for index, text := range raw {
		last := index == len(raw)-1
		if text == "" && !last {
			return nil, invalid("empty segment ('//')")
		}
		if len(text) >= 1 && text[0] == ':' {
			text = "{" + text[1:] + "}"
		} else if len(text) >= 1 && text[0] == '*' {
			text = "{" + text[1:] + "...}"
		}
		if len(text) >= 2 && text[0] == '{' && text[len(text)-1] == '}' {
			name, rest := text[1:len(text)-1], false
			if strings.HasSuffix(name, "...") {
				name, rest = name[:len(name)-3], true
			}
			if !stdRestValidName(name) {
				return nil, invalid(fmt.Sprintf("invalid parameter name '%s'", name))
			}
			if names[name] {
				return nil, invalid(fmt.Sprintf("duplicate parameter '%s'", name))
			}
			names[name] = true
			if rest && !last {
				return nil, invalid(fmt.Sprintf("'{%s...}' must be the last segment", name))
			}
			kind := stdRestParam
			if rest {
				kind = stdRestRest
			}
			segs = append(segs, stdRestSeg{kind: kind, value: name})
			continue
		}
		if strings.ContainsAny(text, "{}") {
			return nil, invalid(fmt.Sprintf("malformed placeholder in segment '%s'", text))
		}
		segs = append(segs, stdRestSeg{kind: stdRestLit, value: text})
	}
	shape := make([]string, len(segs))
	for i, s := range segs {
		switch s.kind {
		case stdRestLit:
			shape[i] = "L" + s.value
		case stdRestParam:
			shape[i] = "P"
		default:
			shape[i] = "R"
		}
	}
	return &stdRestRoute{method: method, path: path, segments: segs, shape: strings.Join(shape, "/")}, nil
}

func (t *stdRestTable) add(method, path, annotation string) error {
	r, err := stdRestParseRoute(method, path)
	if err != nil {
		return err
	}
	for _, other := range t.routes {
		if other.method == method && other.shape == r.shape {
			return NewHostError(fmt.Sprintf("rest: duplicate route %s %s", method, path))
		}
	}
	r.annotation = annotation
	t.routes = append(t.routes, r)
	return nil
}

func stdRestMatchSegments(r *stdRestRoute, raw []string) (map[string]string, bool) {
	params := map[string]string{}
	for i, s := range r.segments {
		if s.kind == stdRestRest {
			if len(raw) <= i {
				return nil, false
			}
			parts := make([]string, 0, len(raw)-i)
			for _, part := range raw[i:] {
				d, ok := stdRestPercentDecode(part, false)
				if !ok {
					return nil, false
				}
				parts = append(parts, d)
			}
			params[s.value] = strings.Join(parts, "/")
			return params, true
		}
		if i >= len(raw) {
			return nil, false
		}
		decoded, ok := stdRestPercentDecode(raw[i], false)
		if !ok {
			return nil, false
		}
		if s.kind == stdRestLit {
			if decoded != s.value {
				return nil, false
			}
		} else {
			if decoded == "" {
				return nil, false
			}
			params[s.value] = decoded
		}
	}
	return params, len(raw) == len(r.segments)
}

func stdRestCompareSpecificity(a, b *stdRestRoute) int {
	n := len(a.segments)
	if len(b.segments) < n {
		n = len(b.segments)
	}
	for i := 0; i < n; i++ {
		if d := int(a.segments[i].kind) - int(b.segments[i].kind); d != 0 {
			return d
		}
	}
	return len(a.segments) - len(b.segments)
}

func (t *stdRestTable) match(method, rawPath string) (route *stdRestRoute, params map[string]string, status int, allow string) {
	raw := strings.Split(strings.TrimPrefix(rawPath, "/"), "/")
	type hit struct {
		route  *stdRestRoute
		params map[string]string
	}
	var hits []hit
	for _, r := range t.routes {
		if p, ok := stdRestMatchSegments(r, raw); ok {
			hits = append(hits, hit{r, p})
		}
	}
	if len(hits) == 0 {
		return nil, nil, 404, ""
	}
	pick := func(verb string) *hit {
		var best *hit
		for i := range hits {
			h := &hits[i]
			if h.route.method != verb {
				continue
			}
			if best == nil || stdRestCompareSpecificity(h.route, best.route) < 0 {
				best = h
			}
		}
		return best
	}
	if chosen := pick(method); chosen != nil {
		return chosen.route, chosen.params, 0, ""
	}
	if method == "HEAD" {
		if chosen := pick("GET"); chosen != nil {
			return chosen.route, chosen.params, 0, ""
		}
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.route.method] = true
		if h.route.method == "GET" {
			seen["HEAD"] = true
		}
	}
	var allowed []string
	for _, m := range stdRestMethods {
		if seen[m] {
			allowed = append(allowed, m)
		}
	}
	return nil, nil, 405, strings.Join(allowed, ", ")
}

type stdRestExchange struct {
	state      map[string]string
	counts     map[string]int64
	method     string
	target     string
	path       string
	reqHeaders [][2]string
	reqBody    string
	params     map[string]string
	query      map[string]string
	status     int
	headers    [][2]string
	body       string
}

func (x *stdRestExchange) setHeader(name, value string) {
	key := strings.ToLower(name)
	for i, h := range x.headers {
		if strings.ToLower(h[0]) == key {
			x.headers[i] = [2]string{name, value}
			return
		}
	}
	x.headers = append(x.headers, [2]string{name, value})
}

type stdRestEntryRef struct {
	index int
	name  string
}

type stdRestDefaultController struct {
	state                  map[string]string
	counts                 map[string]int64
	configPrefix           string
	table                  stdRestTable
	program                *Program
	context                *Context
	registry               *Registry
	bound                  bool
	serving                bool
	stopped                bool
	maxBody                int64
	autoRoutesMaterialized bool
	autoEntryIndex         map[string]stdRestEntryRef
	autoEntryIndices       map[int]bool
	entries                map[string]stdRestEntryRef
	cliOptions             map[string]stdRestEntryRef
}

func stdMethodForIface(iface string) string {
	switch iface {
	case "interface RestGet(RestExchange) -> void":
		return "GET"
	case "interface RestPost(RestExchange) -> void":
		return "POST"
	case "interface RestPut(RestExchange) -> void":
		return "PUT"
	case "interface RestDelete(RestExchange) -> void":
		return "DELETE"
	case "interface RestPatch(RestExchange) -> void":
		return "PATCH"
	case "interface RestHead(RestExchange) -> void":
		return "HEAD"
	case "interface RestOptions(RestExchange) -> void":
		return "OPTIONS"
	default:
		return ""
	}
}

func stdIsRestOrCliIface(iface string) bool {
	return iface == "interface RestHandler(RestExchange) -> void" ||
		iface == "interface CliCommand(RestExchange) -> void" ||
		iface == "interface CliOption(RestExchange) -> void" ||
		stdMethodForIface(iface) != ""
}

func (c *stdRestDefaultController) normalizedPrefix() string {
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

func (c *stdRestDefaultController) materializeAutoRoutes() error {
	if c.autoRoutesMaterialized || c.program == nil {
		return nil
	}
	c.autoRoutesMaterialized = true
	prefix := c.normalizedPrefix()
	for i, e := range c.program.Entries() {
		ann := e.Annotation
		if e.Iface == "interface CliOption(RestExchange) -> void" {
			c.cliOptions[ann] = stdRestEntryRef{i, e.Name}
			if stripped := strings.TrimLeft(ann, "-"); stripped != "" {
				c.cliOptions[stripped] = stdRestEntryRef{i, e.Name}
			}
			c.autoEntryIndices[i] = true
			continue
		}
		var method, path string
		if m := stdMethodForIface(e.Iface); m != "" {
			if sp := strings.IndexByte(ann, ' '); sp >= 0 {
				method, path = ann[:sp], ann[sp+1:]
			} else if strings.HasPrefix(ann, "/") {
				method, path = m, ann
			} else {
				continue
			}
		} else if e.Iface == "interface RestHandler(RestExchange) -> void" {
			if sp := strings.IndexByte(ann, ' '); sp >= 0 {
				method, path = ann[:sp], ann[sp+1:]
			} else if strings.HasPrefix(ann, "/") {
				method, path = "GET", ann
			} else {
				continue
			}
		} else if e.Iface == "interface CliCommand(RestExchange) -> void" {
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
		c.autoEntryIndex[tag] = stdRestEntryRef{i, e.Name}
		c.autoEntryIndices[i] = true
	}
	return nil
}

func (c *stdRestDefaultController) ensureDispatchReady() error {
	if err := c.materializeAutoRoutes(); err != nil {
		return err
	}
	if len(c.entries) == 0 && len(c.autoEntryIndex) > 0 {
		for k, v := range c.autoEntryIndex {
			c.entries[k] = v
		}
		if c.program != nil {
			for i, e := range c.program.Entries() {
				if !stdIsRestOrCliIface(e.Iface) || e.Iface == "interface CliOption(RestExchange) -> void" || c.autoEntryIndices[i] {
					continue
				}
				if _, exists := c.entries[e.Annotation]; !exists {
					c.entries[e.Annotation] = stdRestEntryRef{i, e.Name}
				}
			}
		}
	}
	return nil
}

func (c *stdRestDefaultController) callEntry(index int, x *stdRestExchange) error {
	saved, wasActive := c.registry.SuspendStdRun()
	defer c.registry.ResumeStdRun(saved, wasActive)
	_, err := c.program.Call(index, c.context, c.registry, []Value{Obj("RestExchange", x)})
	return err
}

func (c *stdRestDefaultController) cli(argv []string) (string, error) {
	if !c.bound || c.program == nil {
		return "", NewHostError("rest: no program is bound (run the program through the controller)")
	}
	if err := c.ensureDispatchReady(); err != nil {
		return "", err
	}
	method, body, methodExplicit := "GET", "", false
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
					_, _, st, _ := c.table.match("GET", probePath)
					if !(len(positionals) == 0 && !isSpecial && st != 404) {
						i++
						val = argv[i]
					}
				}
			}
			if key == "method" || key == "X" {
				method, methodExplicit = strings.ToUpper(val), true
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
	if prefix := c.normalizedPrefix(); prefix != "" && !strings.HasPrefix(path, prefix) {
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
		c.state[opt.key] = opt.val
	}
	x := &stdRestExchange{
		state:   c.state,
		counts:  c.counts,
		method:  method,
		target:  path,
		path:    path,
		reqBody: body,
		params:  map[string]string{},
		query:   query,
		status:  200,
	}
	for _, opt := range parsedOpts {
		target, ok := c.cliOptions[opt.flag]
		if !ok {
			target, ok = c.cliOptions[opt.key]
		}
		if ok {
			if err := c.callEntry(target.index, x); err != nil {
				return "", err
			}
		}
	}
	if c.stopped {
		if x.status == 204 || x.status == 304 {
			return "", nil
		}
		return x.body, nil
	}
	rt, params, st, _ := c.table.match(method, path)
	if st == 405 && !methodExplicit {
		if body != "" {
			if r2, p2, s2, _ := c.table.match("POST", path); s2 == 0 {
				x.method, rt, params, st = "POST", r2, p2, 0
			}
		}
		if st == 405 {
			for _, cand := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"} {
				if r2, p2, s2, _ := c.table.match(cand, path); s2 == 0 {
					x.method, rt, params, st = cand, r2, p2, 0
					break
				}
			}
		}
	}
	switch st {
	case 404:
		if len(positionals) == 0 && x.body != "" {
			return x.body, nil
		}
		return "Not Found", nil
	case 405:
		return "Method Not Allowed", nil
	}
	target, found := c.entries[rt.annotation]
	if !found {
		return "Internal Server Error", nil
	}
	x.params = params
	if err := c.callEntry(target.index, x); err != nil {
		return "Internal Server Error", nil
	}
	if x.status == 204 || x.status == 304 {
		return "", nil
	}
	return x.body, nil
}

func registerStdRest(r *Registry) error {
	c := &stdRestDefaultController{
		state:            map[string]string{},
		counts:           map[string]int64{},
		maxBody:          1024 * 1024,
		autoEntryIndex:   map[string]stdRestEntryRef{},
		autoEntryIndices: map[int]bool{},
		entries:          map[string]stdRestEntryRef{},
		cliOptions:       map[string]stdRestEntryRef{},
	}
	r.SetRestHooks(
		func(program *Program, ctx *Context, reg *Registry) (bool, error) {
			c.program, c.context, c.registry = program, ctx, reg
			c.bound, c.serving, c.stopped = true, false, false
			c.table = stdRestTable{}
			c.autoRoutesMaterialized = false
			c.autoEntryIndex = map[string]stdRestEntryRef{}
			c.autoEntryIndices = map[int]bool{}
			c.entries = map[string]stdRestEntryRef{}
			c.cliOptions = map[string]stdRestEntryRef{}
			c.configPrefix = ""
			return true, nil
		},
		func(program *Program, ctx *Context, reg *Registry) error {
			return nil
		},
	)
	none := None()
	dryNone := Function(func([]Value) (Value, error) { return none, nil })
	S := []string{"string"}

	if err := r.registerBuiltinFunction("rest_config", []string{"string", "string"}, "void", func(a []Value) (Value, error) {
		if c.serving {
			return none, NewHostError("rest: routes are frozen once rest_serve has been called")
		}
		c.state[a[0].Str] = a[1].Str
		if a[0].Str == "prefix" || a[0].Str == "base_path" {
			c.configPrefix = a[1].Str
		} else if a[0].Str == "max_body_bytes" && a[1].Str != "" {
			if n, err := strconv.ParseInt(a[1].Str, 10, 64); err == nil && n > 0 {
				c.maxBody = n
			}
		}
		return none, nil
	}, Dry(dryNone)); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("rest_route", []string{"string", "string", "string"}, "void", func(a []Value) (Value, error) {
		if c.serving {
			return none, NewHostError("rest: routes are frozen once rest_serve has been called")
		}
		return none, c.table.add(a[0].Str, a[1].Str, a[2].Str)
	}, Dry(dryNone)); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("rest_serve", nil, "void", func(a []Value) (Value, error) {
		if c.serving {
			return none, NewHostError("rest: rest_serve already called")
		}
		if !c.bound || c.program == nil {
			return none, NewHostError("rest: no program is bound (run the program through the controller)")
		}
		if err := c.materializeAutoRoutes(); err != nil {
			return none, err
		}
		if len(c.table.routes) == 0 {
			return none, NewHostError("rest: no routes registered")
		}
		c.entries = map[string]stdRestEntryRef{}
		for k, v := range c.autoEntryIndex {
			c.entries[k] = v
		}
		for i, e := range c.program.Entries() {
			if !stdIsRestOrCliIface(e.Iface) || e.Iface == "interface CliOption(RestExchange) -> void" || c.autoEntryIndices[i] {
				continue
			}
			if _, dup := c.entries[e.Annotation]; dup {
				return none, NewHostError(fmt.Sprintf("rest: duplicate annotation @%s", e.Annotation))
			}
			c.entries[e.Annotation] = stdRestEntryRef{i, e.Name}
		}
		for _, rt := range c.table.routes {
			if _, ok := c.entries[rt.annotation]; ok {
				continue
			}
			for _, e := range c.program.Entries() {
				if e.Annotation == rt.annotation {
					return none, NewHostError(fmt.Sprintf("rest: entry %s must implement RestHandler(RestExchange) -> void", e.Name))
				}
			}
			return none, NewHostError(fmt.Sprintf("rest: route %s %s references missing annotation @%s", rt.method, rt.path, rt.annotation))
		}
		c.serving = true
		return none, nil
	}, Dry(dryNone)); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("rest_stop", nil, "void", func(a []Value) (Value, error) {
		c.stopped = true
		return none, nil
	}, Dry(dryNone)); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("rest_cli", []string{"StrList"}, "string", func(a []Value) (Value, error) {
		list, ok := a[0].Obj.(*StrList)
		if !ok || list == nil {
			return String(""), NewHostError("rest_cli: invalid StrList")
		}
		out, err := c.cli(list.Values())
		return String(out), err
	}, Dry(Function(func([]Value) (Value, error) { return String(""), nil }))); err != nil {
		return err
	}

	regMethod := func(name string, params []string, returns string, fn func(x *stdRestExchange, a []Value) (Value, error)) error {
		return r.registerPureBuiltinMethod("RestExchange", name, params, returns, func(self any, a []Value) (Value, error) {
			x, ok := self.(*stdRestExchange)
			if !ok || x == nil {
				return None(), NewHostError("invalid exchange")
			}
			return fn(x, a)
		})
	}
	_ = sort.Strings
	_ = io.EOF
	_ = os.Stdin
	if err := regMethod("method", nil, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		return String(x.method), nil
	}); err != nil {
		return err
	}
	if err := regMethod("path", nil, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		return String(x.path), nil
	}); err != nil {
		return err
	}
	if err := regMethod("query", S, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		return String(x.query[a[0].Str]), nil
	}); err != nil {
		return err
	}
	if err := regMethod("has_query", S, "bool", func(x *stdRestExchange, a []Value) (Value, error) {
		_, ok := x.query[a[0].Str]
		return Bool(ok), nil
	}); err != nil {
		return err
	}
	if err := regMethod("param", S, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		return String(x.params[a[0].Str]), nil
	}); err != nil {
		return err
	}
	if err := regMethod("header", S, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		var vals []string
		want := strings.ToLower(a[0].Str)
		for _, h := range x.reqHeaders {
			if strings.ToLower(h[0]) == want {
				vals = append(vals, h[1])
			}
		}
		return String(strings.Join(vals, ", ")), nil
	}); err != nil {
		return err
	}
	if err := regMethod("has_header", S, "bool", func(x *stdRestExchange, a []Value) (Value, error) {
		want := strings.ToLower(a[0].Str)
		for _, h := range x.reqHeaders {
			if strings.ToLower(h[0]) == want {
				return Bool(true), nil
			}
		}
		return Bool(false), nil
	}); err != nil {
		return err
	}
	if err := regMethod("body", nil, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		return String(x.reqBody), nil
	}); err != nil {
		return err
	}
	if err := regMethod("status", []string{"int"}, "void", func(x *stdRestExchange, a []Value) (Value, error) {
		if a[0].Int < 200 || a[0].Int > 599 {
			return none, NewHostError(fmt.Sprintf("rest: status %d out of range 200..599", a[0].Int))
		}
		x.status = int(a[0].Int)
		return none, nil
	}); err != nil {
		return err
	}
	if err := regMethod("set_header", []string{"string", "string"}, "void", func(x *stdRestExchange, a []Value) (Value, error) {
		name, value := a[0].Str, a[1].Str
		bad := name == "" || strings.ContainsAny(value, "\r\n")
		for i := 0; i < len(name) && !bad; i++ {
			bad = name[i] <= 0x20 || name[i] == 0x7f || name[i] == ':'
		}
		if bad {
			return none, NewHostError("rest: invalid response header")
		}
		switch strings.ToLower(name) {
		case "content-length", "connection", "transfer-encoding":
			return none, NewHostError(fmt.Sprintf("rest: response header %s is managed by the server", name))
		}
		x.setHeader(name, value)
		return none, nil
	}); err != nil {
		return err
	}
	if err := regMethod("send", S, "void", func(x *stdRestExchange, a []Value) (Value, error) {
		x.body = a[0].Str
		return none, nil
	}); err != nil {
		return err
	}
	if err := regMethod("json", S, "void", func(x *stdRestExchange, a []Value) (Value, error) {
		x.body = a[0].Str
		x.setHeader("Content-Type", "application/json")
		return none, nil
	}); err != nil {
		return err
	}
	if err := regMethod("get", S, "string", func(x *stdRestExchange, a []Value) (Value, error) {
		return String(x.state[a[0].Str]), nil
	}); err != nil {
		return err
	}
	if err := regMethod("set", []string{"string", "string"}, "void", func(x *stdRestExchange, a []Value) (Value, error) {
		x.state[a[0].Str] = a[1].Str
		return none, nil
	}); err != nil {
		return err
	}
	if err := regMethod("increment", S, "int", func(x *stdRestExchange, a []Value) (Value, error) {
		x.counts[a[0].Str]++
		return Int(x.counts[a[0].Str]), nil
	}); err != nil {
		return err
	}
	return nil
}
