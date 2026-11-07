package rest

import (
	"strings"
	"testing"
)

func wantHostError(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("want error containing %q, got %v", substr, err)
	}
}

func TestRouteTemplateValidShapesParse(t *testing.T) {
	r, err := parseRoute("GET", "/users/{id}/files/{path...}")
	if err != nil {
		t.Fatal(err)
	}
	kinds := []segKind{kindLiteral, kindParam, kindLiteral, kindRest}
	for i, s := range r.segments {
		if s.kind != kinds[i] {
			t.Fatalf("segment %d: kind %v", i, s.kind)
		}
	}
	if r, _ := parseRoute("GET", "/"); len(r.segments) != 1 {
		t.Fatal("root route")
	}
	if r, _ := parseRoute("GET", "/files/"); r.segments[1].value != "" {
		t.Fatal("trailing empty segment")
	}
	if _, err := parseRoute("GET", "/\xc3\xa0/\xa0"); err != nil {
		t.Fatalf("0xA0 is an ordinary byte: %v", err)
	}
}

func TestRouteTemplateInvalidShapesHavePinnedMessages(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"get", "/x", "rest: invalid route get /x: method must be one of GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS"},
		{"GET", "x", "path must start with '/'"},
		{"GET", "/a b", "path must not contain whitespace, control characters, ? or #"},
		{"GET", "/a\x01", "whitespace, control characters"},
		{"GET", "/a\x7f", "whitespace, control characters"},
		{"GET", "/a?b", "whitespace, control characters, ? or #"},
		{"GET", "/a#b", "? or #"},
		{"GET", "/a//b", "empty segment ('//')"},
		{"GET", "/{1x}", "invalid parameter name '1x'"},
		{"GET", "/{}", "invalid parameter name ''"},
		{"GET", "/{...}", "invalid parameter name ''"},
		{"GET", "/{a}/{a}", "duplicate parameter 'a'"},
		{"GET", "/{a...}/x", "'{a...}' must be the last segment"},
		{"GET", "/a{b}", "malformed placeholder in segment 'a{b}'"},
		{"GET", "/{b", "malformed placeholder in segment '{b'"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			_, err := parseRoute(c.method, c.path)
			wantHostError(t, err, c.want)
		})
	}
}

func TestRouteTableDuplicatesAreDetectedByShape(t *testing.T) {
	var tb routeTable
	for _, r := range [][3]string{{"GET", "/a/{x}", "one"}, {"POST", "/a/{x}", "two"}, {"GET", "/a/b", "literal"}} {
		if err := tb.add(r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}
	wantHostError(t, tb.add("GET", "/a/{y}", "three"), "rest: duplicate route GET /a/{y}")
}

func TestRouteTableSpecificityOrdering(t *testing.T) {
	var tb routeTable
	for _, p := range [][2]string{{"/x/{rest...}", "rest"}, {"/x/{p}/z", "param-z"}, {"/x/{p}/{q}", "param-param"}, {"/x/y/{q}", "literal-param"}, {"/x/y/z", "literal-literal"}} {
		if err := tb.add("GET", p[0], p[1]); err != nil {
			t.Fatal(err)
		}
	}
	pick := func(path string) string {
		m := tb.match("GET", path)
		if m.route == nil {
			return "none"
		}
		return m.route.annotation
	}
	for path, want := range map[string]string{"/x/y/z": "literal-literal", "/x/y/w": "literal-param", "/x/a/z": "param-z", "/x/a/b": "param-param", "/x/a/b/c": "rest", "/x/": "rest", "/x": "none"} {
		if got := pick(path); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
	if m := tb.match("GET", "/x"); m.status != 404 {
		t.Fatal(m)
	}
}

func TestRouteTable404And405AndHeadFallback(t *testing.T) {
	var tb routeTable
	tb.add("GET", "/r", "g")
	tb.add("DELETE", "/r", "d")
	tb.add("PUT", "/only-put", "p")
	if m := tb.match("POST", "/r"); m.status != 405 || m.allow != "DELETE, GET, HEAD" {
		t.Fatal(m)
	}
	if m := tb.match("GET", "/only-put"); m.status != 405 || m.allow != "PUT" {
		t.Fatal(m)
	}
	if m := tb.match("GET", "/missing"); m.status != 404 {
		t.Fatal(m)
	}
	if m := tb.match("HEAD", "/r"); m.route == nil || m.route.method != "GET" {
		t.Fatal(m)
	}
	if m := tb.match("HEAD", "/only-put"); m.status != 405 {
		t.Fatal(m)
	}
	tb.add("HEAD", "/r", "h")
	if m := tb.match("HEAD", "/r"); m.route.method != "HEAD" {
		t.Fatal("an explicit HEAD route wins over the GET fallback")
	}
}

func TestRouteMatchingPercentDecoding(t *testing.T) {
	var tb routeTable
	tb.add("GET", "/f/{name}", "f")
	if m := tb.match("GET", "/f/a%2Fb"); m.route == nil || m.params["name"] != "a/b" {
		t.Fatal(m)
	}
	for path, why := range map[string]string{"/f/a%2Fb/c": "encoded slash stays in one segment", "/f/%zz": "invalid escapes never match", "/f/": "an empty segment is not a {param}"} {
		if m := tb.match("GET", path); m.status != 404 {
			t.Errorf("%s: %s: %+v", path, why, m)
		}
	}
}

func TestPercentDecode(t *testing.T) {
	if got, ok := percentDecode("%C3%B6", false); !ok || got != "\xc3\xb6" {
		t.Fatalf("decoding yields bytes, got %q", got)
	}
	if got, _ := percentDecode("a+b", true); got != "a b" {
		t.Fatal(got)
	}
	if got, _ := percentDecode("a+b", false); got != "a+b" {
		t.Fatal(got)
	}
	if got, ok := percentDecode("%ff%00", false); !ok || got != "\xff\x00" {
		t.Fatalf("non-UTF-8 bytes survive: %q", got)
	}
	for _, bad := range []string{"%4", "%", "%zz", "%4g", "abc%"} {
		if _, ok := percentDecode(bad, false); ok {
			t.Errorf("%q must be invalid", bad)
		}
	}
}
