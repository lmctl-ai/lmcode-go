package lmcode

import (
	"encoding/base64"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
)

const maxBytes = 64 * 1024 * 1024

// StrLen returns the byte length, including NUL and invalid UTF-8 bytes.
func StrLen(s string) int64 { return int64(len(s)) }

// StrBase64Encode returns canonical padded RFC 4648 encoding of raw bytes.
func StrBase64Encode(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// StrBase64Decode accepts only canonical padded RFC 4648 input.
func StrBase64Decode(s string) (string, error) {
	for i := range s {
		c := s[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '=') {
			return "", NewHostError("str_base64_decode: invalid encoding")
		}
	}
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || base64.StdEncoding.EncodeToString(b) != s {
		return "", NewHostError("str_base64_decode: invalid encoding")
	}
	return string(b), nil
}

// StrHexEncode returns lowercase hexadecimal for raw bytes.
func StrHexEncode(s string) string { return hex.EncodeToString([]byte(s)) }

// StrHexDecode decodes an even number of ASCII hexadecimal digits.
func StrHexDecode(s string) (string, error) {
	if len(s)%2 != 0 {
		return "", NewHostError("str_hex_decode: invalid encoding")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", NewHostError("str_hex_decode: invalid encoding")
	}
	return string(b), nil
}

// StrStartsWith tests a byte prefix; an empty prefix matches.
func StrStartsWith(s, p string) bool { return strings.HasPrefix(s, p) }

// StrEndsWith tests a byte suffix; an empty suffix matches.
func StrEndsWith(s, p string) bool { return strings.HasSuffix(s, p) }

// StrContains tests for a byte substring; an empty needle matches.
func StrContains(s, n string) bool { return strings.Contains(s, n) }

// StrFind returns the first byte offset, or -1 if absent; an empty needle returns 0.
func StrFind(s, n string) int64 { return int64(strings.Index(s, n)) }

// StrSubstr slices bytes, clamping a negative start to zero and the count to the remaining bytes.
// Nonpositive counts and starts beyond the end return empty strings.
func StrSubstr(s string, start, count int64) string {
	if start < 0 {
		start = 0
	}
	if count <= 0 || uint64(start) >= uint64(len(s)) {
		return ""
	}
	off := int(start)
	n := int(count)
	if uint64(count) > uint64(len(s)-off) {
		n = len(s) - off
	}
	return s[off : off+n]
}

// StrLower maps ASCII A-Z to a-z without decoding or changing other bytes.
func StrLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// StrUpper maps ASCII a-z to A-Z without decoding or changing other bytes.
func StrUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// StrTrim strips ASCII space and bytes 0x09 through 0x0d from both ends.
func StrTrim(s string) string {
	ws := func(c byte) bool { return c == ' ' || (c >= '\t' && c <= '\r') }
	a, b := 0, len(s)
	for a < b && ws(s[a]) {
		a++
	}
	for b > a && ws(s[b-1]) {
		b--
	}
	return s[a:b]
}

// StrRepeat repeats bytes, rejecting negative counts and results over 64 MiB before allocation.
func StrRepeat(s string, count int64) (string, error) {
	if len(s) == 0 && count >= 0 {
		return "", nil
	}
	if count < 0 || (len(s) > 0 && uint64(count) > uint64(maxBytes/len(s))) {
		return "", NewHostError("str_repeat: count must be nonnegative and result at most 67108864 bytes")
	}
	return strings.Repeat(s, int(count)), nil
}

// StrReplace replaces non-overlapping matches after preflighting a 64 MiB result cap.
// An empty search is a no-op; even no-op results must fit the cap.
func StrReplace(s, from, to string) (string, error) {
	if from == "" {
		if len(s) > maxBytes {
			return "", NewHostError("str_replace: result must be at most 67108864 bytes")
		}
		return s, nil
	}
	size, at := 0, 0
	for {
		i := strings.Index(s[at:], from)
		if i < 0 {
			break
		}
		p := at + i
		if p-at > maxBytes-size || len(to) > maxBytes-size-(p-at) {
			return "", NewHostError("str_replace: result must be at most 67108864 bytes")
		}
		size += p - at + len(to)
		at = p + len(from)
	}
	if len(s)-at > maxBytes-size {
		return "", NewHostError("str_replace: result must be at most 67108864 bytes")
	}
	return strings.ReplaceAll(s, from, to), nil
}

// StrIsInt reports whether the entire token is a signed decimal int64, without whitespace.
func StrIsInt(s string) bool { _, ok := parseInt(s); return ok }

// StrToInt parses a signed decimal int64, returning zero on invalid syntax or overflow.
func StrToInt(s string) int64 {
	v, ok := parseInt(s)
	if !ok {
		return 0
	}
	return v
}
func parseInt(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	if i == len(s) {
		return 0, false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	v, e := strconv.ParseInt(s, 10, 64)
	return v, e == nil
}

// StrIsFloat validates decimal syntax and finite binary64 range, rejecting nonzero underflow to zero.
func StrIsFloat(s string) bool { _, ok := parseFloat(s); return ok }

// StrToFloat parses a decimal float, returning zero for invalid or out-of-range tokens.
// Representable subnormals and signed zero are preserved independently of process locale.
func StrToFloat(s string) float64 { v, _ := parseFloat(s); return v }
func parseFloat(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	digits, nonzero := false, false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		digits = true
		nonzero = nonzero || s[i] != '0'
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			digits = true
			nonzero = nonzero || s[i] != '0'
			i++
		}
	}
	if !digits {
		return 0, false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		e := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == e {
			return 0, false
		}
	}
	if i != len(s) {
		return 0, false
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil {
		return 0, false
	}
	if math.IsInf(v, 0) || (v == 0 && nonzero) {
		return 0, false
	}
	return v, true
}

type globWork struct{ steps uint64 }

func (w *globWork) step() error {
	if w.steps == 1<<27 {
		return NewHostError("str_match: pattern exceeds the 2^27 work limit")
	}
	w.steps++
	return nil
}
func globToken(p string, i int, c byte, next *int, w *globWork) (bool, error) {
	if p[i] == '\\' && i+1 < len(p) {
		*next = i + 2
		return p[i+1] == c, nil
	}
	if p[i] == '?' {
		*next = i + 1
		return true, nil
	}
	if p[i] == '[' {
		q, neg := i+1, false
		if q < len(p) && p[q] == '!' {
			neg = true
			q++
		}
		first, hit := q, false
		for q < len(p) && (p[q] != ']' || q == first) {
			if e := w.step(); e != nil {
				return false, e
			}
			read := func() byte {
				if p[q] == '\\' && q+1 < len(p) {
					q++
				}
				b := p[q]
				q++
				return b
			}
			lo := read()
			if q+1 < len(p) && p[q] == '-' && p[q+1] != ']' {
				q++
				hi := read()
				if lo <= hi && c >= lo && c <= hi {
					hit = true
				}
			} else if lo == c {
				hit = true
			}
		}
		if q < len(p) {
			*next = q + 1
			return hit != neg, nil
		}
	}
	*next = i + 1
	return p[i] == c, nil
}

// StrMatch matches a whole byte string against *, ?, classes/ranges, [!class], and backslash escapes.
// It uses constant auxiliary memory and a 2^27 work cap, charged for each matcher iteration,
// class entry scanned and trailing star skipped. Unterminated classes and trailing backslashes are literal.
func StrMatch(text, pattern string) (bool, error) {
	work := globWork{}
	t, p, star, starT := 0, 0, -1, 0
	for t < len(text) {
		if err := work.step(); err != nil {
			return false, err
		}
		if p < len(pattern) && pattern[p] == '*' {
			star, starT = p, t
			p++
			continue
		}
		if p < len(pattern) {
			var next int
			matched, err := globToken(pattern, p, text[t], &next, &work)
			if err != nil {
				return false, err
			}
			if matched {
				p = next
				t++
				continue
			}
		}
		if star < 0 {
			return false, nil
		}
		p = star + 1
		starT++
		t = starT
	}
	for p < len(pattern) && pattern[p] == '*' {
		if err := work.step(); err != nil {
			return false, err
		}
		p++
	}
	return p == len(pattern), nil
}

// RegisterString binds the optional byte string and encoding catalog functions.
func RegisterString(r *Registry) error {
	entries := []struct {
		n, t string
		p    []string
		f    Function
	}{
		{"str_base64_encode", "string", []string{"string"}, func(a []Value) (Value, error) { return String(StrBase64Encode(a[0].Str)), nil }},
		{"str_base64_decode", "string", []string{"string"}, func(a []Value) (Value, error) {
			v, e := StrBase64Decode(a[0].Str)
			if e != nil {
				return None(), e
			}
			return String(v), nil
		}},
		{"str_hex_encode", "string", []string{"string"}, func(a []Value) (Value, error) { return String(StrHexEncode(a[0].Str)), nil }},
		{"str_hex_decode", "string", []string{"string"}, func(a []Value) (Value, error) {
			v, e := StrHexDecode(a[0].Str)
			if e != nil {
				return None(), e
			}
			return String(v), nil
		}},
		{"str_starts_with", "bool", []string{"string", "string"}, func(a []Value) (Value, error) {
			return Bool(StrStartsWith(a[0].Str, a[1].Str)), nil
		}},
		{"str_ends_with", "bool", []string{"string", "string"}, func(a []Value) (Value, error) { return Bool(StrEndsWith(a[0].Str, a[1].Str)), nil }},
		{"str_contains", "bool", []string{"string", "string"}, func(a []Value) (Value, error) { return Bool(StrContains(a[0].Str, a[1].Str)), nil }},
		{"str_len", "int", []string{"string"}, func(a []Value) (Value, error) { return Int(StrLen(a[0].Str)), nil }},
		{"str_substr", "string", []string{"string", "int", "int"}, func(a []Value) (Value, error) {
			return Str(StrSubstr(a[0].Str, a[1].Int, a[2].Int)), nil
		}},
		{"str_lower", "string", []string{"string"}, func(a []Value) (Value, error) { return Str(StrLower(a[0].Str)), nil }},
		{"str_upper", "string", []string{"string"}, func(a []Value) (Value, error) { return Str(StrUpper(a[0].Str)), nil }},
		{"str_trim", "string", []string{"string"}, func(a []Value) (Value, error) { return Str(StrTrim(a[0].Str)), nil }},
		{"str_repeat", "string", []string{"string", "int"}, func(a []Value) (Value, error) {
			v, e := StrRepeat(a[0].Str, a[1].Int)
			if e != nil {
				return None(), e
			}
			return Str(v), nil
		}},
		{"str_to_int", "int", []string{"string"}, func(a []Value) (Value, error) { return Int(StrToInt(a[0].Str)), nil }},
		{"str_is_int", "bool", []string{"string"}, func(a []Value) (Value, error) { return Bool(StrIsInt(a[0].Str)), nil }},
		{"str_to_float", "float", []string{"string"}, func(a []Value) (Value, error) { return Float(StrToFloat(a[0].Str)), nil }},
		{"str_is_float", "bool", []string{"string"}, func(a []Value) (Value, error) { return Bool(StrIsFloat(a[0].Str)), nil }},
		{"str_find", "int", []string{"string", "string"}, func(a []Value) (Value, error) { return Int(StrFind(a[0].Str, a[1].Str)), nil }},
		{"str_replace", "string", []string{"string", "string", "string"}, func(a []Value) (Value, error) {
			v, e := StrReplace(a[0].Str, a[1].Str, a[2].Str)
			if e != nil {
				return None(), e
			}
			return Str(v), nil
		}},
		{"str_match", "bool", []string{"string", "string"}, func(a []Value) (Value, error) {
			v, e := StrMatch(a[0].Str, a[1].Str)
			if e != nil {
				return None(), e
			}
			return Bool(v), nil
		}},
	}
	for _, e := range entries {
		if err := r.registerBuiltinFunction(e.n, e.p, e.t, e.f); err != nil {
			return err
		}
	}
	return nil
}
