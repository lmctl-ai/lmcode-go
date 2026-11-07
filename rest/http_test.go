package rest

import (
	"reflect"
	"strings"
	"testing"
)

func TestHTTPRequestHeads(t *testing.T) {
	req, status := parseHead("GET /a?b=1 HTTP/1.1\r\nHost: x\r\nX-A:  \t v \t ")
	want := Request{Method: "GET", Target: "/a?b=1", Headers: [][2]string{{"Host", "x"}, {"X-A", "v"}}}
	if status != 0 || !reflect.DeepEqual(req, want) {
		t.Fatalf("%+v %d", req, status)
	}
	if _, status := parseHead("POST /x HTTP/1.0"); status != 0 {
		t.Fatal("HTTP/1.0 is accepted")
	}
	bad := []string{"garbage", "GET /a HTTP/2.0", "GET a HTTP/1.1", "GET /a#f HTTP/1.1", "get /a HTTP/1.1", "GET /a HTTP/1.1\r\nNoColon", "GET /a HTTP/1.1\r\n: v",
		"GET /a HTTP/1.1\r\nBad Name: v", "GET /a b HTTP/1.1", "GET  HTTP/1.1", "GET /a  HTTP/1.1", "GET /a\x01 HTTP/1.1", "GET /a\x7f HTTP/1.1", "", " /a HTTP/1.1",
		"GET /a HTTP/1.1\r\nX: a\x01b", "GET /a HTTP/1.1\r\nX: a\rb", "GET /a HTTP/1.1\r\nX: a\x7f", "GET /a HTTP/1.1\r\n\x01X: v"}
	for _, b := range bad {
		if _, status := parseHead(b); status != 400 {
			t.Errorf("%q: status %d, want 400", b, status)
		}
	}
}

func TestHTTPWhitespaceIsAsciiOnly(t *testing.T) {
	req, status := parseHead("GET /\xc3\xa0 HTTP/1.1\r\nX-\xa0: \xa0v\xa0")
	if status != 0 || req.Target != "/\xc3\xa0" || req.Headers[0] != [2]string{"X-\xa0", "\xa0v\xa0"} {
		t.Fatalf("0xA0 is an ordinary byte: %+v %d", req, status)
	}
}

func TestHTTPContentLengthRules(t *testing.T) {
	h := func(pairs ...string) (hs [][2]string) {
		for i := 0; i < len(pairs); i += 2 {
			hs = append(hs, [2]string{pairs[i], pairs[i+1]})
		}
		return
	}
	if n, s := bodyLength(nil); n != 0 || s != 0 {
		t.Fatal(n, s)
	}
	if n, s := bodyLength(h("content-length", "12")); n != 12 || s != 0 {
		t.Fatal(n, s)
	}
	if n, s := bodyLength(h("Content-Length", "1", "content-length", "1")); n != 1 || s != 0 {
		t.Fatal(n, s)
	}
	if _, s := bodyLength(h("Content-Length", "1", "content-length", "2")); s != 400 {
		t.Fatal(s)
	}
	for _, v := range []string{"-1", "1.5", "abc", "", strings.Repeat("1", 16), "١", " 1"} {
		if _, s := bodyLength(h("Content-Length", v)); s != 400 {
			t.Errorf("%q: %d", v, s)
		}
	}
	if _, s := bodyLength(h("Transfer-Encoding", "chunked")); s != 501 {
		t.Fatal(s)
	}
	if _, s := bodyLength(h("Content-Length", "x", "TRANSFER-ENCODING", "chunked")); s != 501 {
		t.Fatal("501 wins over a bad Content-Length")
	}
}

func TestHTTPResponseEncoding(t *testing.T) {
	got := string(encodeResponse(Response{Status: 200, Headers: [][2]string{{"A", "b"}}, Body: "hi"}, false))
	if got != "HTTP/1.1 200 OK\r\nA: b\r\nContent-Length: 2\r\n\r\nhi" {
		t.Fatalf("%q", got)
	}
	got = string(encodeResponse(Response{Status: 299, Body: "hi"}, true))
	if got != "HTTP/1.1 299 OK\r\nContent-Length: 2\r\n\r\n" {
		t.Fatalf("HEAD keeps the GET Content-Length: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "connection") {
		t.Fatal("no Connection header")
	}
}

func TestHTTPTakeRequestFraming(t *testing.T) {
	const post = "POST /sum HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello"
	if got := takeRequest([]byte("GET /a HTTP/1.1\r\nHo"), 100); !got.need {
		t.Fatal("incomplete head waits")
	}
	if got := takeRequest([]byte(post[:len(post)-1]), 100); !got.need {
		t.Fatal("incomplete body waits")
	}
	got := takeRequest([]byte(post+"GET /next"), 100)
	if got.need || got.fatal != 0 || got.request.Body != "hello" || got.consumed != len(post) {
		t.Fatalf("%+v", got)
	}
	if got := takeRequest([]byte(post), 4); got.fatal != 413 {
		t.Fatalf("413 is decided from the header: %+v", got)
	}
	if got := takeRequest([]byte("POST /sum HTTP/1.1\r\nContent-Length: 9999\r\n\r\n"), 100); got.fatal != 413 {
		t.Fatalf("body unread: %+v", got)
	}
	header := func(n int) string {
		return "GET /a HTTP/1.1\r\nX: " + strings.Repeat("a", n-len("X: ")-len("GET /a HTTP/1.1\r\n"))
	}
	if got := takeRequest([]byte(header(maxHeaderBytes)+"\r\n\r\n"), 100); got.fatal != 0 || got.need {
		t.Fatalf("a header section of exactly 16 KiB is fine: %+v", got)
	}
	if got := takeRequest([]byte(header(maxHeaderBytes+1)+"\r\n\r\n"), 100); got.fatal != 431 {
		t.Fatalf("%+v", got)
	}
	if got := takeRequest([]byte(header(maxHeaderBytes+1)), 100); got.fatal != 431 {
		t.Fatalf("431 is detected before the head ends: %+v", got)
	}
	if got := takeRequest([]byte("garbage\r\n\r\n"), 100); got.fatal != 400 {
		t.Fatal(got)
	}
	if got := takeRequest([]byte("\r\n\r\n"), 100); got.fatal != 400 {
		t.Fatal(got)
	}
}

func TestHTTPReasonPhrases(t *testing.T) {
	for status, want := range map[int]string{200: "OK", 204: "No Content", 405: "Method Not Allowed", 431: "Request Header Fields Too Large", 299: "OK", 399: "Redirect", 499: "Client Error", 599: "Server Error"} {
		if got := reasonPhrase(status); got != want {
			t.Errorf("%d: %q", status, got)
		}
	}
}
