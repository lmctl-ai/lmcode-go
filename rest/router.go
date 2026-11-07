package rest

import (
	"fmt"
	"sort"
	"strings"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

// Methods are the HTTP verbs a route may use.
var Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

func hostErr(format string, args ...any) error {
	return lmcode.NewHostError(fmt.Sprintf(format, args...))
}

// isSpace reports the ASCII-only "whitespace" of the spec: bytes 0x00-0x20 and 0x7f.
func isSpace(c byte) bool { return c <= 0x20 || c == 0x7f }

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

// percentDecode decodes a byte string; ok is false on an invalid escape. plus turns '+' into a space (queries).
func percentDecode(text string, plus bool) (string, bool) {
	if !strings.ContainsAny(text, "%+") {
		return text, true
	}
	out := make([]byte, 0, len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '%':
			if i+2 >= len(text) || !isHex(text[i+1]) || !isHex(text[i+2]) {
				return "", false
			}
			out = append(out, unhex(text[i+1])<<4|unhex(text[i+2]))
			i += 2
		case plus && c == '+':
			out = append(out, ' ')
		default:
			out = append(out, c)
		}
	}
	return string(out), true
}

type segKind int

const (
	kindLiteral segKind = iota
	kindParam
	kindRest
)

type segment struct {
	kind  segKind
	value string // literal text, or parameter name
}

type route struct {
	method     string
	path       string
	segments   []segment
	shape      string
	annotation string
}

func validName(s string) bool {
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

func parseRoute(method, path string) (*route, error) {
	invalid := func(reason string) error {
		return hostErr("rest: invalid route %s %s: %s", method, path, reason)
	}
	known := false
	for _, m := range Methods {
		known = known || m == method
	}
	if !known {
		return nil, invalid("method must be one of " + strings.Join(Methods, ", "))
	}
	if !strings.HasPrefix(path, "/") {
		return nil, invalid("path must start with '/'")
	}
	for i := 0; i < len(path); i++ {
		if isSpace(path[i]) || path[i] == '?' || path[i] == '#' {
			return nil, invalid("path must not contain whitespace, control characters, ? or #")
		}
	}
	raw := strings.Split(path[1:], "/")
	names := map[string]bool{}
	segs := make([]segment, 0, len(raw))
	for index, text := range raw {
		last := index == len(raw)-1
		if text == "" && !last {
			return nil, invalid("empty segment ('//')")
		}
		if len(text) >= 2 && text[0] == '{' && text[len(text)-1] == '}' {
			name, rest := text[1:len(text)-1], false
			if strings.HasSuffix(name, "...") {
				name, rest = name[:len(name)-3], true
			}
			if !validName(name) {
				return nil, invalid(fmt.Sprintf("invalid parameter name '%s'", name))
			}
			if names[name] {
				return nil, invalid(fmt.Sprintf("duplicate parameter '%s'", name))
			}
			names[name] = true
			if rest && !last {
				return nil, invalid(fmt.Sprintf("'{%s...}' must be the last segment", name))
			}
			kind := kindParam
			if rest {
				kind = kindRest
			}
			segs = append(segs, segment{kind: kind, value: name})
			continue
		}
		if strings.ContainsAny(text, "{}") {
			return nil, invalid(fmt.Sprintf("malformed placeholder in segment '%s'", text))
		}
		segs = append(segs, segment{kind: kindLiteral, value: text})
	}
	shape := make([]string, len(segs))
	for i, s := range segs {
		switch s.kind {
		case kindLiteral:
			shape[i] = "L" + s.value
		case kindParam:
			shape[i] = "P"
		default:
			shape[i] = "R"
		}
	}
	return &route{method: method, path: path, segments: segs, shape: strings.Join(shape, "/")}, nil
}

// compareSpecificity is negative when a is more specific than b (segment by segment, literal < {p} < {p...}).
func compareSpecificity(a, b *route) int {
	n := min(len(a.segments), len(b.segments))
	for i := 0; i < n; i++ {
		if d := int(a.segments[i].kind) - int(b.segments[i].kind); d != 0 {
			return d
		}
	}
	return len(a.segments) - len(b.segments)
}

func matchSegments(r *route, raw []string) (map[string]string, bool) {
	params := map[string]string{}
	for i, s := range r.segments {
		if s.kind == kindRest {
			if len(raw) <= i {
				return nil, false
			}
			parts := make([]string, 0, len(raw)-i)
			for _, part := range raw[i:] {
				d, ok := percentDecode(part, false)
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
		decoded, ok := percentDecode(raw[i], false)
		if !ok {
			return nil, false
		}
		if s.kind == kindLiteral {
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

// routeTable holds the registered routes in registration order.
type routeTable struct{ routes []*route }

func (t *routeTable) add(method, path, annotation string) error {
	r, err := parseRoute(method, path)
	if err != nil {
		return err
	}
	for _, other := range t.routes {
		if other.method == method && other.shape == r.shape {
			return hostErr("rest: duplicate route %s %s", method, path)
		}
	}
	r.annotation = annotation
	t.routes = append(t.routes, r)
	return nil
}

// matchResult is a route and its parameters, or a 404/405 verdict (status != 0) with the Allow value.
type matchResult struct {
	route  *route
	params map[string]string
	status int
	allow  string
}

// match resolves rawPath (undecoded, without the query).
func (t *routeTable) match(method, rawPath string) matchResult {
	raw := strings.Split(strings.TrimPrefix(rawPath, "/"), "/")
	type hit struct {
		route  *route
		params map[string]string
	}
	var hits []hit
	for _, r := range t.routes {
		if params, ok := matchSegments(r, raw); ok {
			hits = append(hits, hit{r, params})
		}
	}
	if len(hits) == 0 {
		return matchResult{status: 404}
	}
	best := func(verb string) *hit {
		var found []hit
		for _, h := range hits {
			if h.route.method == verb {
				found = append(found, h)
			}
		}
		if len(found) == 0 {
			return nil
		}
		sort.SliceStable(found, func(i, j int) bool { return compareSpecificity(found[i].route, found[j].route) < 0 })
		return &found[0]
	}
	direct := best(method)
	if direct == nil && method == "HEAD" {
		direct = best("GET")
	}
	if direct != nil {
		return matchResult{route: direct.route, params: direct.params}
	}
	allow := map[string]bool{}
	for _, h := range hits {
		allow[h.route.method] = true
	}
	if allow["GET"] {
		allow["HEAD"] = true
	}
	verbs := make([]string, 0, len(allow))
	for v := range allow {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)
	return matchResult{status: 405, allow: strings.Join(verbs, ", ")}
}
