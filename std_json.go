package lmcode

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const jsonMaximumDepth = 256

type jsonKind uint8

const (
	jsonNull jsonKind = iota
	jsonBool
	jsonInt
	jsonFloat
	jsonString
	jsonArray
	jsonObject
)

type jsonMember struct {
	key    string
	rawKey string
	value  *JSON
}

// JSON is an immutable node in a parsed JSON document. Child handles borrow
// nodes from their root and remain valid for the current registry run.
type JSON struct {
	kind     jsonKind
	boolean  bool
	integer  int64
	floating float64
	text     string
	raw      string
	array    []*JSON
	object   []jsonMember
}

func (j *JSON) Kind() string {
	switch j.kind {
	case jsonNull:
		return "null"
	case jsonBool:
		return "bool"
	case jsonInt:
		return "int"
	case jsonFloat:
		return "float"
	case jsonString:
		return "string"
	case jsonArray:
		return "array"
	case jsonObject:
		return "object"
	default:
		return "null"
	}
}

func (j *JSON) Len() (int64, error) {
	if j.kind == jsonArray {
		return int64(len(j.array)), nil
	}
	if j.kind == jsonObject {
		return int64(len(j.object)), nil
	}
	return 0, NewHostError("Json.len: expected array or object")
}

func (j *JSON) At(index int64) (*JSON, error) {
	if j.kind != jsonArray {
		return nil, NewHostError("Json.at: expected array")
	}
	if index < 0 || index >= int64(len(j.array)) {
		return nil, NewHostError("Json.at: index out of range: " + formatInt(index))
	}
	return j.array[int(index)], nil
}

func (j *JSON) Get(key string) (*JSON, error) {
	if j.kind != jsonObject {
		return nil, NewHostError("Json.get: expected object")
	}
	for _, member := range j.object {
		if member.key == key {
			return member.value, nil
		}
	}
	return nil, NewHostError("Json.get: missing key")
}

func (j *JSON) Has(key string) (bool, error) {
	if j.kind != jsonObject {
		return false, NewHostError("Json.has: expected object")
	}
	for _, member := range j.object {
		if member.key == key {
			return true, nil
		}
	}
	return false, nil
}

func (j *JSON) Keys() ([]string, error) {
	if j.kind != jsonObject {
		return nil, NewHostError("Json.keys: expected object")
	}
	keys := make([]string, len(j.object))
	for i, member := range j.object {
		keys[i] = member.key
	}
	return keys, nil
}

func (j *JSON) AsString() (string, error) {
	if j.kind != jsonString {
		return "", NewHostError("Json.as_string: expected string")
	}
	return j.text, nil
}

func (j *JSON) AsInt() (int64, error) {
	if j.kind != jsonInt {
		return 0, NewHostError("Json.as_int: expected int")
	}
	return j.integer, nil
}

func (j *JSON) AsFloat() (float64, error) {
	if j.kind == jsonFloat {
		return j.floating, nil
	}
	if j.kind == jsonInt {
		return float64(j.integer), nil
	}
	return 0, NewHostError("Json.as_float: expected number")
}

func (j *JSON) AsBool() (bool, error) {
	if j.kind != jsonBool {
		return false, NewHostError("Json.as_bool: expected bool")
	}
	return j.boolean, nil
}

// ToJSON emits compact JSON, preserving original string/number tokens and key order.
func (j *JSON) ToJSON() string {
	var out strings.Builder
	writeJSON(&out, j)
	return out.String()
}

func writeJSON(out *strings.Builder, value *JSON) {
	switch value.kind {
	case jsonNull:
		out.WriteString("null")
	case jsonBool:
		if value.boolean {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case jsonInt, jsonFloat, jsonString:
		out.WriteString(value.raw)
	case jsonArray:
		out.WriteByte('[')
		for i, child := range value.array {
			if i != 0 {
				out.WriteByte(',')
			}
			writeJSON(out, child)
		}
		out.WriteByte(']')
	case jsonObject:
		out.WriteByte('{')
		for i, member := range value.object {
			if i != 0 {
				out.WriteByte(',')
			}
			out.WriteString(member.rawKey)
			out.WriteByte(':')
			writeJSON(out, member.value)
		}
		out.WriteByte('}')
	}
}

type jsonParser struct {
	source string
	pos    int
}

func jsonParseError(reason string, position int) error {
	return NewHostError("json_parse: " + reason + " at byte " + strconv.Itoa(position))
}

func validateJSONUTF8(source string) error {
	for i := 0; i < len(source); {
		_, size := utf8.DecodeRuneInString(source[i:])
		if size == 1 && source[i] >= utf8.RuneSelf {
			return jsonParseError("invalid UTF-8", i)
		}
		i += size
	}
	return nil
}

func parseJSON(source string) (*JSON, error) {
	if err := validateJSONUTF8(source); err != nil {
		return nil, err
	}
	if strings.HasPrefix(source, "\xef\xbb\xbf") {
		return nil, jsonParseError("BOM is not allowed", 0)
	}
	parser := jsonParser{source: source}
	value, err := parser.value(0)
	if err != nil {
		return nil, err
	}
	parser.whitespace()
	if parser.pos != len(source) {
		return nil, jsonParseError("trailing input", parser.pos)
	}
	return value, nil
}

// ParseJSON parses strict JSON and retains raw token spellings for ToJSON.
func ParseJSON(source string) (*JSON, error) { return parseJSON(source) }

func (p *jsonParser) whitespace() {
	for p.pos < len(p.source) {
		switch p.source[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) take(ch byte) bool {
	if p.pos < len(p.source) && p.source[p.pos] == ch {
		p.pos++
		return true
	}
	return false
}

func (p *jsonParser) literal(word string) error {
	if !strings.HasPrefix(p.source[p.pos:], word) {
		return jsonParseError("invalid literal", p.pos)
	}
	p.pos += len(word)
	return nil
}

func (p *jsonParser) hex4(at int) (uint32, error) {
	if at+4 > len(p.source) {
		return 0, jsonParseError("invalid Unicode escape", at)
	}
	var result uint32
	for i := 0; i < 4; i++ {
		c := p.source[at+i]
		var digit byte
		switch {
		case c >= '0' && c <= '9':
			digit = c - '0'
		case c >= 'a' && c <= 'f':
			digit = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			digit = c - 'A' + 10
		default:
			return 0, jsonParseError("invalid Unicode escape", at+i)
		}
		result = (result << 4) | uint32(digit)
	}
	return result, nil
}

func (p *jsonParser) stringToken() (string, string, error) {
	start := p.pos
	if !p.take('"') {
		return "", "", jsonParseError("expected string", p.pos)
	}
	var decoded strings.Builder
	for p.pos < len(p.source) {
		c := p.source[p.pos]
		p.pos++
		if c == '"' {
			return decoded.String(), p.source[start:p.pos], nil
		}
		if c < 0x20 {
			return "", "", jsonParseError("control byte in string", p.pos-1)
		}
		if c != '\\' {
			decoded.WriteByte(c)
			continue
		}
		if p.pos >= len(p.source) {
			return "", "", jsonParseError("unterminated escape", p.pos)
		}
		escape := p.source[p.pos]
		p.pos++
		switch escape {
		case '"', '\\', '/':
			decoded.WriteByte(escape)
		case 'b':
			decoded.WriteByte('\b')
		case 'f':
			decoded.WriteByte('\f')
		case 'n':
			decoded.WriteByte('\n')
		case 'r':
			decoded.WriteByte('\r')
		case 't':
			decoded.WriteByte('\t')
		case 'u':
			if p.pos+4 > len(p.source) {
				return "", "", jsonParseError("invalid Unicode escape", p.pos)
			}
			codePoint, err := p.hex4(p.pos)
			if err != nil {
				return "", "", err
			}
			p.pos += 4
			if codePoint >= 0xd800 && codePoint <= 0xdbff {
				if p.pos+6 > len(p.source) || p.source[p.pos:p.pos+2] != "\\u" {
					return "", "", jsonParseError("unpaired high surrogate", p.pos)
				}
				low, err := p.hex4(p.pos + 2)
				if err != nil {
					return "", "", err
				}
				if low < 0xdc00 || low > 0xdfff {
					return "", "", jsonParseError("unpaired high surrogate", p.pos)
				}
				p.pos += 6
				codePoint = 0x10000 + ((codePoint - 0xd800) << 10) + (low - 0xdc00)
			} else if codePoint >= 0xdc00 && codePoint <= 0xdfff {
				return "", "", jsonParseError("unpaired low surrogate", p.pos-4)
			}
			decoded.WriteRune(rune(codePoint))
		default:
			return "", "", jsonParseError("invalid escape", p.pos-1)
		}
	}
	return "", "", jsonParseError("unterminated string", p.pos)
}

func (p *jsonParser) number() (*JSON, error) {
	start := p.pos
	if p.source[p.pos] == '-' {
		p.pos++
	}
	if p.pos >= len(p.source) || p.source[p.pos] < '0' || p.source[p.pos] > '9' {
		return nil, jsonParseError("invalid number", p.pos)
	}
	if p.source[p.pos] == '0' {
		p.pos++
	} else {
		for p.pos < len(p.source) && p.source[p.pos] >= '0' && p.source[p.pos] <= '9' {
			p.pos++
		}
	}
	isFloat := false
	if p.pos < len(p.source) && p.source[p.pos] == '.' {
		isFloat = true
		p.pos++
		if p.pos >= len(p.source) || p.source[p.pos] < '0' || p.source[p.pos] > '9' {
			return nil, jsonParseError("invalid number", p.pos)
		}
		for p.pos < len(p.source) && p.source[p.pos] >= '0' && p.source[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.source) && (p.source[p.pos] == 'e' || p.source[p.pos] == 'E') {
		isFloat = true
		p.pos++
		if p.pos < len(p.source) && (p.source[p.pos] == '+' || p.source[p.pos] == '-') {
			p.pos++
		}
		if p.pos >= len(p.source) || p.source[p.pos] < '0' || p.source[p.pos] > '9' {
			return nil, jsonParseError("invalid number", p.pos)
		}
		for p.pos < len(p.source) && p.source[p.pos] >= '0' && p.source[p.pos] <= '9' {
			p.pos++
		}
	}
	raw := p.source[start:p.pos]
	value := &JSON{raw: raw}
	if !isFloat {
		integer, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, jsonParseError("integer out of range", start)
		}
		value.kind, value.integer = jsonInt, integer
		return value, nil
	}
	floating, _ := strconv.ParseFloat(raw, 64) // Grammar was checked above; finite underflow is allowed.
	if math.IsInf(floating, 0) || math.IsNaN(floating) {
		return nil, jsonParseError("nonfinite number", start)
	}
	value.kind, value.floating = jsonFloat, floating
	return value, nil
}

func (p *jsonParser) value(depth int) (*JSON, error) {
	p.whitespace()
	if p.pos >= len(p.source) {
		return nil, jsonParseError("expected value", p.pos)
	}
	start := p.pos
	value := &JSON{}
	switch p.source[p.pos] {
	case 'n':
		if err := p.literal("null"); err != nil {
			return nil, err
		}
		value.kind = jsonNull
	case 't':
		if err := p.literal("true"); err != nil {
			return nil, err
		}
		value.kind, value.boolean = jsonBool, true
	case 'f':
		if err := p.literal("false"); err != nil {
			return nil, err
		}
		value.kind = jsonBool
	case '"':
		text, raw, err := p.stringToken()
		if err != nil {
			return nil, err
		}
		value.kind, value.text, value.raw = jsonString, text, raw
	case '[':
		if depth+1 > jsonMaximumDepth {
			return nil, jsonParseError("maximum depth exceeded", p.pos)
		}
		value.kind = jsonArray
		p.pos++
		p.whitespace()
		if p.take(']') {
			return value, nil
		}
		for {
			child, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			value.array = append(value.array, child)
			p.whitespace()
			if p.take(']') {
				break
			}
			if !p.take(',') {
				return nil, jsonParseError("expected comma", p.pos)
			}
			p.whitespace()
		}
	case '{':
		if depth+1 > jsonMaximumDepth {
			return nil, jsonParseError("maximum depth exceeded", p.pos)
		}
		value.kind = jsonObject
		p.pos++
		p.whitespace()
		if p.take('}') {
			return value, nil
		}
		seen := make(map[string]struct{})
		for {
			p.whitespace()
			keyPosition := p.pos
			if p.pos >= len(p.source) || p.source[p.pos] != '"' {
				return nil, jsonParseError("object key must be a string", p.pos)
			}
			key, rawKey, err := p.stringToken()
			if err != nil {
				return nil, err
			}
			if _, exists := seen[key]; exists {
				return nil, jsonParseError("duplicate object key", keyPosition)
			}
			seen[key] = struct{}{}
			p.whitespace()
			if !p.take(':') {
				return nil, jsonParseError("expected colon", p.pos)
			}
			child, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			value.object = append(value.object, jsonMember{key, rawKey, child})
			p.whitespace()
			if p.take('}') {
				break
			}
			if !p.take(',') {
				return nil, jsonParseError("expected comma", p.pos)
			}
			p.whitespace()
		}
	default:
		if p.source[p.pos] == '-' || p.source[p.pos] >= '0' && p.source[p.pos] <= '9' {
			return p.number()
		}
		return nil, jsonParseError("unexpected value", p.pos)
	}
	if value.kind == jsonString && value.raw == "" {
		value.raw = p.source[start:p.pos]
	}
	return value, nil
}

// RegisterJSON adds strict JSON parsing and immutable navigation to a registry.
// Parsed roots and keys() lists share the collection arena supplied by RegisterStd.
func RegisterJSON(r *Registry, arenas ...*Collections) error {
	arena := groupCollections(r, arenas)
	if err := r.registerBuiltinFunction("json_parse", []string{"string"}, "Json", func(args []Value) (Value, error) {
		parsed, err := parseJSON(args[0].Str)
		if err != nil {
			return None(), err
		}
		return arena.object("Json", parsed)
	}); err != nil {
		return err
	}
	methods := []struct {
		name    string
		params  []string
		returns string
		fn      Method
	}{
		{"kind", nil, "string", func(self any, _ []Value) (Value, error) { return String(self.(*JSON).Kind()), nil }},
		{"len", nil, "int", func(self any, _ []Value) (Value, error) {
			n, err := self.(*JSON).Len()
			if err != nil {
				return None(), err
			}
			return Int(n), nil
		}},
		{"at", []string{"int"}, "Json", func(self any, a []Value) (Value, error) {
			child, err := self.(*JSON).At(a[0].Int)
			if err != nil {
				return None(), err
			}
			return Object("Json", child), nil
		}},
		{"get", []string{"string"}, "Json", func(self any, a []Value) (Value, error) {
			child, err := self.(*JSON).Get(a[0].Str)
			if err != nil {
				return None(), err
			}
			return Object("Json", child), nil
		}},
		{"has", []string{"string"}, "bool", func(self any, a []Value) (Value, error) {
			ok, err := self.(*JSON).Has(a[0].Str)
			if err != nil {
				return None(), err
			}
			return Bool(ok), nil
		}},
		{"keys", nil, "StrList", func(self any, _ []Value) (Value, error) {
			keys, err := self.(*JSON).Keys()
			if err != nil {
				return None(), err
			}
			return arena.object("StrList", &StrList{values: keys})
		}},
		{"as_string", nil, "string", func(self any, _ []Value) (Value, error) {
			v, err := self.(*JSON).AsString()
			if err != nil {
				return None(), err
			}
			return String(v), nil
		}},
		{"as_int", nil, "int", func(self any, _ []Value) (Value, error) {
			v, err := self.(*JSON).AsInt()
			if err != nil {
				return None(), err
			}
			return Int(v), nil
		}},
		{"as_float", nil, "float", func(self any, _ []Value) (Value, error) {
			v, err := self.(*JSON).AsFloat()
			if err != nil {
				return None(), err
			}
			return Float(v), nil
		}},
		{"as_bool", nil, "bool", func(self any, _ []Value) (Value, error) {
			v, err := self.(*JSON).AsBool()
			if err != nil {
				return None(), err
			}
			return Bool(v), nil
		}},
		{"to_json", nil, "string", func(self any, _ []Value) (Value, error) { return String(self.(*JSON).ToJSON()), nil }},
	}
	for _, method := range methods {
		if err := r.registerBuiltinMethod("Json", method.name, method.params, method.returns, method.fn); err != nil {
			return err
		}
	}
	return nil
}
