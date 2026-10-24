package lmcode

import (
	"fmt"
	"math"
)

// Kind identifies the variant of a Value.
type Kind int

const (
	KindVoid Kind = iota
	KindInt
	KindFloat
	KindStr
	KindBool
	KindObj
)

// Synonyms matching reference names.
const (
	VOID  = KindVoid
	INT   = KindInt
	FLOAT = KindFloat
	STR   = KindStr
	BOOL  = KindBool
	OBJ   = KindObj
)

// Value represents a typed DSL value.
type Value struct {
	Kind  Kind
	Int   int64
	Float float64
	Str   string
	Bool  bool
	Obj   any
	Type  string // schema type name of Obj
}

// None returns a void/none Value.
func None() Value {
	return Value{Kind: KindVoid}
}

// Integer returns an INT Value.
func Integer(v int64) Value {
	return Value{Kind: KindInt, Int: v}
}

// Int is a synonym for Integer.
func Int(v int64) Value {
	return Value{Kind: KindInt, Int: v}
}

// Real returns a FLOAT Value.
func Real(v float64) Value {
	return Value{Kind: KindFloat, Float: v}
}

// Float is a synonym for Real.
func Float(v float64) Value {
	return Value{Kind: KindFloat, Float: v}
}

// String returns a STR Value.
func String(v string) Value {
	return Value{Kind: KindStr, Str: v}
}

// Str is a synonym for String.
func Str(v string) Value {
	return Value{Kind: KindStr, Str: v}
}

// Boolean returns a BOOL Value.
func Boolean(v bool) Value {
	return Value{Kind: KindBool, Bool: v}
}

// Bool is a synonym for Boolean.
func Bool(v bool) Value {
	return Value{Kind: KindBool, Bool: v}
}

// Object returns an OBJ Value holding a host object and its schema type name.
func Object(typeName string, obj any) Value {
	return Value{Kind: KindObj, Type: typeName, Obj: obj}
}

// Obj is a synonym for Object.
func Obj(typeName string, obj any) Value {
	return Value{Kind: KindObj, Type: typeName, Obj: obj}
}

// KindName returns the schema name of this value's kind.
func (v Value) KindName() string {
	switch v.Kind {
	case KindVoid:
		return "void"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"
	case KindStr:
		return "string"
	case KindBool:
		return "bool"
	case KindObj:
		return "object"
	default:
		return "unknown"
	}
}

// AsInt returns the int64 value or an error if not an INT.
func (v Value) AsInt() (int64, error) {
	if v.Kind != KindInt {
		return 0, NewError(RUN_ERROR, fmt.Sprintf("expected int, got %s", v.KindName()))
	}
	return v.Int, nil
}

// AsFloat returns the float64 value or an error if not a FLOAT.
func (v Value) AsFloat() (float64, error) {
	if v.Kind != KindFloat {
		return 0, NewError(RUN_ERROR, fmt.Sprintf("expected float, got %s", v.KindName()))
	}
	return v.Float, nil
}

// AsStr returns the string value or an error if not a STR.
func (v Value) AsStr() (string, error) {
	if v.Kind != KindStr {
		return "", NewError(RUN_ERROR, fmt.Sprintf("expected string, got %s", v.KindName()))
	}
	return v.Str, nil
}

// AsBool returns the bool value or an error if not a BOOL.
func (v Value) AsBool() (bool, error) {
	if v.Kind != KindBool {
		return false, NewError(RUN_ERROR, fmt.Sprintf("expected bool, got %s", v.KindName()))
	}
	return v.Bool, nil
}

// AsObj returns the host object or an error if not an OBJ.
func (v Value) AsObj() (any, error) {
	if v.Kind != KindObj {
		return nil, NewError(RUN_ERROR, fmt.Sprintf("expected object, got %s", v.KindName()))
	}
	return v.Obj, nil
}

// FormatG15 formats a float64 matching C's "%.15g" output across all platforms.
func FormatG15(x float64) string {
	if math.IsNaN(x) {
		return "nan"
	}
	if math.IsInf(x, 1) {
		return "inf"
	}
	if math.IsInf(x, -1) {
		return "-inf"
	}
	if x == 0 {
		if math.Signbit(x) {
			return "-0"
		}
		return "0"
	}
	return fmt.Sprintf("%.15g", x)
}
