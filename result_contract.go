package lmcode

import (
	"math"
	"strconv"
)

// ResultContract is immutable host-owned schema metadata. The zero value permits
// any valid JSON. It is never taken from an invocation manifest.
type ResultContract struct {
	typ      string
	nullable bool
}

func (c ResultContract) Type() string   { return c.typ }
func (c ResultContract) Nullable() bool { return c.nullable }
func jsonMemberValue(n *JSON, key string) *JSON {
	if n != nil {
		for _, m := range n.object {
			if m.key == key {
				return m.value
			}
		}
	}
	return nil
}
func jsonInteger(n *JSON) (int64, bool) {
	if n == nil {
		return 0, false
	}
	if n.kind == jsonInt {
		return n.integer, n.integer >= -resultSafeInteger && n.integer <= resultSafeInteger
	}
	if n.kind == jsonFloat && !math.IsNaN(n.floating) && math.Abs(n.floating) <= float64(resultSafeInteger) && math.Trunc(n.floating) == n.floating {
		return int64(n.floating), true
	}
	return 0, false
}
func ResultContractFromSchema(raw []byte) (ResultContract, error) {
	var c ResultContract
	bad := func(s string) (ResultContract, error) {
		return c, NewError(LOAD_BAD_SCHEMA, "lmcode result contract: "+s)
	}
	n, err := ParseJSON(string(raw))
	if err != nil {
		return bad(err.Error())
	}
	if n.kind != jsonObject {
		return bad("schema must be an object")
	}
	v, ok := jsonInteger(jsonMemberValue(n, "dsl.schema"))
	if !ok || v != 2 {
		return bad("requires schema 2")
	}
	r := jsonMemberValue(n, "result")
	if r == nil {
		return c, nil
	}
	if r.kind != jsonObject {
		return bad("result must be an object")
	}
	for _, m := range r.object {
		if m.key != "type" && m.key != "nullable" {
			return bad("unknown key " + m.key)
		}
	}
	t := jsonMemberValue(r, "type")
	if t == nil || t.kind != jsonString {
		return bad("type must be a string")
	}
	switch t.text {
	case "null", "bool", "int", "float", "string", "Json", "StrList", "IntList", "FloatList", "StrMap", "IntMap", "FloatMap":
	default:
		return bad("unknown type " + t.text)
	}
	c.typ = t.text
	if nullable := jsonMemberValue(r, "nullable"); nullable != nil {
		if c.typ == "null" {
			return bad("type null forbids nullable")
		}
		if nullable.kind != jsonBool {
			return bad("nullable must be bool")
		}
		c.nullable = nullable.boolean
	}
	return c, nil
}
func resultMismatch(path, expected, got string) error {
	return NewHostError("lmcode return: result type mismatch at " + path + ": expected " + expected + "; got " + got)
}
func primitiveResult(n *JSON, t string) bool {
	if t == "int" {
		_, ok := jsonInteger(n)
		return ok
	}
	return t == "float" && (n.kind == jsonInt || n.kind == jsonFloat) || t == "bool" && n.kind == jsonBool || t == "string" && n.kind == jsonString
}
func (c ResultContract) validate(n *JSON, path string) error {
	if c.typ == "" {
		return nil
	}
	if n == nil {
		n = &JSON{kind: jsonNull}
	}
	if n.kind == jsonNull {
		if c.nullable || c.typ == "null" {
			return nil
		}
		return resultMismatch(path, c.typ, "null")
	}
	if c.typ == "Json" || primitiveResult(n, c.typ) {
		return nil
	}
	list := c.typ == "StrList" || c.typ == "IntList" || c.typ == "FloatList"
	obj := c.typ == "StrMap" || c.typ == "IntMap" || c.typ == "FloatMap"
	if list && n.kind == jsonArray || obj && n.kind == jsonObject {
		element := "float"
		if c.typ == "StrList" || c.typ == "StrMap" {
			element = "string"
		}
		if c.typ == "IntList" || c.typ == "IntMap" {
			element = "int"
		}
		for i, v := range n.array {
			if !primitiveResult(v, element) {
				return resultMismatch(path+"/"+strconv.Itoa(i), element, v.Kind())
			}
		}
		for _, m := range n.object {
			if !primitiveResult(m.value, element) {
				return resultMismatch(resultPointer(path, m.key), element, m.value.Kind())
			}
		}
		return nil
	}
	return resultMismatch(path, c.typ, n.Kind())
}
func resultSource(v Value) string {
	switch v.Kind {
	case KindVoid:
		return "null"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindBool:
		return "bool"
	case KindStr:
		return "string"
	case KindObj:
		return v.Type
	}
	return "handle"
}
func resultAssignable(actual, expected string) bool {
	return actual == expected || actual == "int" && expected == "float" || actual == "IntList" && expected == "FloatList" || actual == "IntMap" && expected == "FloatMap"
}
func (c ResultContract) validateSource(v Value) error {
	if c.typ == "" || c.typ == "Json" || v.Kind == KindVoid || v.Kind == KindObj && v.Type == "Json" {
		return nil
	}
	actual := resultSource(v)
	if !resultAssignable(actual, c.typ) {
		return resultMismatch("", c.typ, actual)
	}
	return nil
}
func (c ResultContract) validateFieldSource(key string, v Value) error {
	if v.Kind == KindObj && v.Type == "Json" {
		return nil
	}
	element := ""
	switch c.typ {
	case "IntMap":
		element = "int"
	case "FloatMap":
		element = "float"
	case "StrMap":
		element = "string"
	}
	if element != "" && !resultAssignable(resultSource(v), element) {
		return resultMismatch(resultPointer("", key), element, resultSource(v))
	}
	return nil
}
