package lmcode

import (
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const resultSafeInteger int64 = 9007199254740991

// ResultSink belongs to one logical invocation, outside the VM collection arena.
// Keep it across Run/Call regions; Reset only before the next program. Like the
// registry, a sink must not be shared by concurrent invocations.
type ResultSink struct{ root *JSON }

func (s *ResultSink) Reset()   { s.root = nil }
func (s *ResultSink) SetNull() { s.Reset() }
func (s *ResultSink) Set(value Value) error {
	owned, err := copyResultValue(value, "")
	if err != nil {
		return err
	}
	s.root = owned
	return nil
}
func (s *ResultSink) fieldKey(key string) error {
	if s.root != nil && s.root.kind != jsonNull && s.root.kind != jsonObject {
		return NewHostError("lmcode return: field assignment requires object or null")
	}
	if !utf8.ValidString(key) {
		return resultInvalid("", "utf8")
	}
	return nil
}
func (s *ResultSink) Put(key string, value Value) error {
	if err := s.fieldKey(key); err != nil {
		return err
	}
	owned, err := copyResultValue(value, resultPointer("", key))
	if err != nil {
		return err
	}
	s.putOwned(key, owned)
	return nil
}
func (s *ResultSink) PutNull(key string) error {
	if err := s.fieldKey(key); err != nil {
		return err
	}
	s.putOwned(key, &JSON{kind: jsonNull})
	return nil
}
func (s *ResultSink) putOwned(key string, value *JSON) {
	if s.root == nil || s.root.kind == jsonNull {
		s.root = &JSON{kind: jsonObject}
	}
	for i := range s.root.object {
		if s.root.object[i].key == key {
			s.root.object[i].value = value
			return
		}
	}
	s.root.object = append(s.root.object, jsonMember{key: strings.Clone(key), rawKey: resultQuote(key), value: value})
}
func (s *ResultSink) JSON() string {
	if s.root == nil {
		return "null"
	}
	return s.root.ToJSON()
}
func ResultFrame(s *ResultSink) string {
	return "LMCODE_RETURN_BEGIN\n" + s.JSON() + "\nLMCODE_RETURN_END\n"
}

// WriteResultFrame is explicit: call only after body, entries AND application
// validation succeed. Error paths must not call it. Use an LF-preserving writer.
func WriteResultFrame(w io.Writer, s *ResultSink) error {
	frame := ResultFrame(s)
	n, err := io.WriteString(w, frame)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}

// SetResultSink configures a host-owned sink; nil restores missing-sink errors.
// Configure outside execution, under the existing registry concurrency discipline.
func (r *Registry) SetResultSink(s *ResultSink) error {
	if !r.stdInstalled {
		return NewHostError("std: registry has standard library disabled")
	}
	r.runState.resultSink = s
	return nil
}
func registerResults(r *Registry) error {
	for _, typ := range []string{"int", "float", "bool", "string", "Json", "StrList", "IntList", "FloatList", "StrMap", "IntMap", "FloatMap"} {
		if err := r.Function("lmcode_return_set", []string{typ}, "void", func(a []Value) (Value, error) {
			if r.runState.resultSink == nil {
				return None(), NewHostError("lmcode return: no result sink")
			}
			return None(), r.runState.resultSink.Set(a[0])
		}); err != nil {
			return err
		}
		if err := r.Function("lmcode_return_put", []string{"string", typ}, "void", func(a []Value) (Value, error) {
			if r.runState.resultSink == nil {
				return None(), NewHostError("lmcode return: no result sink")
			}
			return None(), r.runState.resultSink.Put(a[0].Str, a[1])
		}); err != nil {
			return err
		}
	}
	if err := r.Function("lmcode_return_null", nil, "void", func(a []Value) (Value, error) {
		if r.runState.resultSink == nil {
			return None(), NewHostError("lmcode return: no result sink")
		}
		r.runState.resultSink.SetNull()
		return None(), nil
	}); err != nil {
		return err
	}
	return r.Function("lmcode_return_put_null", []string{"string"}, "void", func(a []Value) (Value, error) {
		if r.runState.resultSink == nil {
			return None(), NewHostError("lmcode return: no result sink")
		}
		return None(), r.runState.resultSink.PutNull(a[0].Str)
	})
}
func resultInvalid(path, reason string) error {
	return NewHostError("lmcode return: invalid JSON value at " + path + ": " + reason)
}
func resultPointer(parent, key string) string {
	return parent + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}
func resultFloat(value float64, path string) (*JSON, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, resultInvalid(path, "nonfinite")
	}
	if math.Trunc(value) == value && math.Abs(value) > float64(resultSafeInteger) {
		return nil, resultInvalid(path, "unsafe_integer")
	}
	if value == 0 {
		value = 0
	}
	// This is deliberately separate from str(float)'s lossy 15-digit formatter.
	return &JSON{kind: jsonFloat, floating: value, raw: strconv.FormatFloat(value, 'g', -1, 64)}, nil
}
func resultInt(value int64, path string) (*JSON, error) {
	if value > resultSafeInteger || value < -resultSafeInteger {
		return nil, resultInvalid(path, "unsafe_integer")
	}
	return &JSON{kind: jsonInt, integer: value, raw: strconv.FormatInt(value, 10)}, nil
}
func resultString(value, path string) (*JSON, error) {
	if !utf8.ValidString(value) {
		return nil, resultInvalid(path, "utf8")
	}
	return &JSON{kind: jsonString, text: strings.Clone(value), raw: resultQuote(value)}, nil
}
func resultQuote(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	const hex = "0123456789abcdef"
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch c {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\b':
			b.WriteString("\\b")
		case '\f':
			b.WriteString("\\f")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if c < 32 {
				b.WriteString("\\u00")
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&15])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

var resultNumberPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// Classify the mathematical decimal before binary64 rounding, without allocating
// exponent-sized integers. The exponent bound is derived from token length.
func resultRawNumber(raw, path string) error {
	if raw == "" {
		return nil
	}
	if !resultNumberPattern.MatchString(raw) {
		return resultInvalid(path, "handle")
	}
	mantissa, exponent := raw, ""
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		mantissa, exponent = raw[:i], raw[i+1:]
	}
	mantissa = strings.TrimPrefix(mantissa, "-")
	fractional := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fractional = len(mantissa) - i - 1
		mantissa = mantissa[:i] + mantissa[i+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return nil
	}
	negative := strings.HasPrefix(exponent, "-")
	exponent = strings.TrimLeft(exponent, "+-")
	exp, bound := 0, len(raw)+16
	for _, c := range exponent {
		if exp > bound/10 || exp*10+int(c-'0') > bound {
			if !negative {
				return resultInvalid(path, "unsafe_integer")
			}
			return nil
		}
		exp = exp*10 + int(c-'0')
	}
	scale := exp - fractional
	if negative {
		scale = -exp - fractional
	}
	if scale < 0 {
		cut := -scale
		if cut >= len(digits) {
			return nil
		}
		for _, c := range digits[len(digits)-cut:] {
			if c != '0' {
				return nil
			}
		}
		digits = digits[:len(digits)-cut]
		scale = 0
	}
	const safe = "9007199254740991"
	if len(digits) > len(safe) || scale > len(safe)-len(digits) {
		return resultInvalid(path, "unsafe_integer")
	}
	digits += strings.Repeat("0", scale)
	if len(digits) == len(safe) && digits > safe {
		return resultInvalid(path, "unsafe_integer")
	}
	return nil
}
func copyResultJSON(j *JSON, path string, active map[*JSON]bool) (*JSON, error) {
	if j == nil {
		return nil, resultInvalid(path, "handle")
	}
	if active[j] {
		return nil, resultInvalid(path, "cycle")
	}
	active[j] = true
	defer delete(active, j)
	switch j.kind {
	case jsonNull:
		return &JSON{kind: jsonNull}, nil
	case jsonBool:
		return &JSON{kind: jsonBool, boolean: j.boolean}, nil
	case jsonInt:
		if err := resultRawNumber(j.raw, path); err != nil {
			return nil, err
		}
		return resultInt(j.integer, path)
	case jsonFloat:
		out, err := resultFloat(j.floating, path)
		if err != nil {
			return nil, err
		}
		if j.floating == 0 {
			for _, c := range strings.Split(strings.ToLower(j.raw), "e")[0] {
				if c >= '1' && c <= '9' {
					return nil, resultInvalid(path, "underflow")
				}
			}
		}
		if err := resultRawNumber(j.raw, path); err != nil {
			return nil, err
		}
		return out, nil
	case jsonString:
		return resultString(j.text, path)
	case jsonArray:
		out := &JSON{kind: jsonArray}
		for i, child := range j.array {
			value, err := copyResultJSON(child, path+"/"+strconv.Itoa(i), active)
			if err != nil {
				return nil, err
			}
			out.array = append(out.array, value)
		}
		return out, nil
	case jsonObject:
		out := &JSON{kind: jsonObject}
		seen := map[string]bool{}
		for _, m := range j.object {
			if !utf8.ValidString(m.key) {
				return nil, resultInvalid(path, "utf8")
			}
			p := resultPointer(path, m.key)
			if seen[m.key] {
				return nil, resultInvalid(p, "handle")
			}
			seen[m.key] = true
			value, err := copyResultJSON(m.value, p, active)
			if err != nil {
				return nil, err
			}
			out.object = append(out.object, jsonMember{key: strings.Clone(m.key), rawKey: resultQuote(m.key), value: value})
		}
		return out, nil
	}
	return nil, resultInvalid(path, "handle")
}
func copyResultValue(v Value, path string) (*JSON, error) {
	switch v.Kind {
	case KindVoid:
		return &JSON{kind: jsonNull}, nil
	case KindInt:
		return resultInt(v.Int, path)
	case KindFloat:
		return resultFloat(v.Float, path)
	case KindBool:
		return &JSON{kind: jsonBool, boolean: v.Bool}, nil
	case KindStr:
		return resultString(v.Str, path)
	case KindObj:
		array := &JSON{kind: jsonArray}
		object := &JSON{kind: jsonObject}
		seen := map[string]bool{}
		add := func(key string, value Value) error {
			if !utf8.ValidString(key) {
				return resultInvalid(path, "utf8")
			}
			if seen[key] {
				return resultInvalid(resultPointer(path, key), "handle")
			}
			seen[key] = true
			owned, err := copyResultValue(value, resultPointer(path, key))
			if err != nil {
				return err
			}
			object.object = append(object.object, jsonMember{key: strings.Clone(key), rawKey: resultQuote(key), value: owned})
			return nil
		}
		list := func(values []Value) (*JSON, error) {
			for i, value := range values {
				owned, err := copyResultValue(value, path+"/"+strconv.Itoa(i))
				if err != nil {
					return nil, err
				}
				array.array = append(array.array, owned)
			}
			return array, nil
		}
		switch v.Type {
		case "Json":
			if j, ok := v.Obj.(*JSON); ok {
				return copyResultJSON(j, path, map[*JSON]bool{})
			}
		case "StrList":
			if l, ok := v.Obj.(*StrList); ok && l != nil {
				values := make([]Value, len(l.values))
				for i, x := range l.values {
					values[i] = String(x)
				}
				return list(values)
			}
		case "IntList":
			if l, ok := v.Obj.(*IntList); ok && l != nil {
				values := make([]Value, len(l.values))
				for i, x := range l.values {
					values[i] = Int(x)
				}
				return list(values)
			}
		case "FloatList":
			if l, ok := v.Obj.(*FloatList); ok && l != nil {
				values := make([]Value, len(l.values))
				for i, x := range l.values {
					values[i] = Float(x)
				}
				return list(values)
			}
		case "StrMap":
			if m, ok := v.Obj.(*StrMap); ok && m != nil {
				for _, e := range m.Entries() {
					if err := add(e.Key, String(e.Value)); err != nil {
						return nil, err
					}
				}
				return object, nil
			}
		case "IntMap":
			if m, ok := v.Obj.(*IntMap); ok && m != nil {
				for _, e := range m.Entries() {
					if err := add(e.Key, Int(e.Value)); err != nil {
						return nil, err
					}
				}
				return object, nil
			}
		case "FloatMap":
			if m, ok := v.Obj.(*FloatMap); ok && m != nil {
				for _, e := range m.Entries() {
					if err := add(e.Key, Float(e.Value)); err != nil {
						return nil, err
					}
				}
				return object, nil
			}
		}
	}
	return nil, resultInvalid(path, "handle")
}
