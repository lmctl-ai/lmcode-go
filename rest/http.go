package rest

import (
	"bytes"
	"strconv"
	"strings"
)

// A small HTTP/1.1 subset (spec/std-rest.md "HTTP subset"): request framing and response encoding.

// maxHeaderBytes bounds the header section (without the terminating blank line).
const maxHeaderBytes = 16 * 1024

// Request is one parsed request; Headers keep their wire order and spelling.
type Request struct {
	Method, Target string
	Headers        [][2]string
	Body           string
}

// Response is what a handler (or a generated error) produced.
type Response struct {
	Status  int
	Headers [][2]string
	Body    string
}

var reasons = map[int]string{200: "OK", 201: "Created", 202: "Accepted", 204: "No Content", 301: "Moved Permanently", 302: "Found", 304: "Not Modified", 400: "Bad Request", 401: "Unauthorized", 403: "Forbidden", 404: "Not Found", 405: "Method Not Allowed", 408: "Request Timeout", 409: "Conflict", 413: "Payload Too Large", 415: "Unsupported Media Type", 422: "Unprocessable Entity", 429: "Too Many Requests", 431: "Request Header Fields Too Large", 500: "Internal Server Error", 501: "Not Implemented", 502: "Bad Gateway", 503: "Service Unavailable"}

func reasonPhrase(status int) string {
	if r, ok := reasons[status]; ok {
		return r
	}
	switch {
	case status < 300:
		return "OK"
	case status < 400:
		return "Redirect"
	case status < 500:
		return "Client Error"
	}
	return "Server Error"
}

var errorBodies = map[int]string{
	400: `{"error":"bad request"}`, 404: `{"error":"not found"}`, 405: `{"error":"method not allowed"}`,
	413: `{"error":"payload too large"}`, 431: `{"error":"request header too large"}`,
	500: `{"error":"internal error"}`, 501: `{"error":"not implemented"}`,
}

func errorResponse(status int, extra ...[2]string) Response {
	headers := append([][2]string{{"Content-Type", "application/json"}}, extra...)
	return Response{Status: status, Headers: headers, Body: errorBodies[status]}
}

// asciiLower lowercases A-Z only, leaving every other byte untouched.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// parseHead parses the header section (without the terminating blank line); status != 0 is the error status.
func parseHead(text string) (req Request, status int) {
	lines := strings.Split(text, "\r\n")
	line := lines[0]
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 || !(strings.HasSuffix(line, " HTTP/1.0") || strings.HasSuffix(line, " HTTP/1.1")) || len(line) < sp+1+1+9 {
		return req, 400
	}
	method, target := line[:sp], line[sp+1:len(line)-9]
	for i := 0; i < len(method); i++ {
		if method[i] < 'A' || method[i] > 'Z' {
			return req, 400
		}
	}
	if target == "" || target[0] != '/' || strings.IndexByte(target, '#') >= 0 {
		return req, 400
	}
	for i := 0; i < len(target); i++ {
		if isSpace(target[i]) {
			return req, 400
		}
	}
	req.Method, req.Target = method, target
	for _, l := range lines[1:] {
		colon := strings.IndexByte(l, ':')
		if colon <= 0 {
			return req, 400
		}
		for i := 0; i < colon; i++ {
			if isSpace(l[i]) {
				return req, 400
			}
		}
		for i := 0; i < len(l); i++ {
			if c := l[i]; c <= 0x08 || c >= 0x0a && c <= 0x1f || c == 0x7f {
				return req, 400
			}
		}
		req.Headers = append(req.Headers, [2]string{l[:colon], strings.Trim(l[colon+1:], " \t")})
	}
	return req, 0
}

// bodyLength returns Content-Length (or 0); status != 0 is an error. Transfer-Encoding is unsupported (501).
func bodyLength(headers [][2]string) (length int64, status int) {
	var values []string
	for _, h := range headers {
		switch asciiLower(h[0]) {
		case "transfer-encoding":
			return 0, 501
		case "content-length":
			values = append(values, h[1])
		}
	}
	if len(values) == 0 {
		return 0, 0
	}
	for _, v := range values[1:] {
		if v != values[0] {
			return 0, 400
		}
	}
	v := values[0]
	if len(v) < 1 || len(v) > 15 {
		return 0, 400
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return 0, 400
		}
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n, 0
}

// encodeResponse renders status line, headers, Content-Length and (unless head) the body.
func encodeResponse(r Response, head bool) []byte {
	var b bytes.Buffer
	b.WriteString("HTTP/1.1 " + strconv.Itoa(r.Status) + " " + reasonPhrase(r.Status) + "\r\n")
	for _, h := range r.Headers {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("Content-Length: " + strconv.Itoa(len(r.Body)) + "\r\n\r\n")
	if !head {
		b.WriteString(r.Body)
	}
	return b.Bytes()
}

// taken is the outcome of takeRequest: need more bytes, a fatal framing status, or a request and its byte length.
type taken struct {
	need     bool
	fatal    int
	request  Request
	consumed int
}

// takeRequest frames one request off the front of buf. A fatal framing error ends serving.
func takeRequest(buf []byte, maxBody int64) taken {
	end := bytes.Index(buf, []byte("\r\n\r\n"))
	if end < 0 {
		if len(buf) > maxHeaderBytes {
			return taken{fatal: 431}
		}
		return taken{need: true}
	}
	if end > maxHeaderBytes {
		return taken{fatal: 431}
	}
	req, status := parseHead(string(buf[:end]))
	if status != 0 {
		return taken{fatal: status}
	}
	size, status := bodyLength(req.Headers)
	if status != 0 {
		return taken{fatal: status}
	}
	if size > maxBody {
		return taken{fatal: 413}
	}
	start := end + 4
	if int64(len(buf)-start) < size {
		return taken{need: true}
	}
	req.Body = string(buf[start : start+int(size)])
	return taken{request: req, consumed: start + int(size)}
}
