package lmcode

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
)

// Bytecode opcodes (v5).
const (
	OpPushInt     byte = 0x01
	OpPushStr     byte = 0x02
	OpPushBool    byte = 0x03
	OpLoadLocal   byte = 0x04
	OpStoreLocal  byte = 0x05
	OpLoadCtx     byte = 0x06
	OpCallFn      byte = 0x07
	OpCallMethod  byte = 0x08
	OpEq          byte = 0x09
	OpNe          byte = 0x0A
	OpNot         byte = 0x0B
	OpJump        byte = 0x0C
	OpJumpIfFalse byte = 0x0D
	OpJumpIfTrue  byte = 0x0E
	OpDup         byte = 0x0F
	OpPop         byte = 0x10
	OpHalt        byte = 0x11

	// v2: floats, typed arithmetic and ordering, explicit widening
	OpPushFloat byte = 0x12
	OpAddI      byte = 0x13
	OpSubI      byte = 0x14
	OpMulI      byte = 0x15
	OpDivI      byte = 0x16
	OpAddF      byte = 0x17
	OpSubF      byte = 0x18
	OpMulF      byte = 0x19
	OpDivF      byte = 0x1A
	OpNegI      byte = 0x1B
	OpNegF      byte = 0x1C
	OpLtI       byte = 0x1D
	OpLeI       byte = 0x1E
	OpGtI       byte = 0x1F
	OpGeI       byte = 0x20
	OpLtF       byte = 0x21
	OpLeF       byte = 0x22
	OpGtF       byte = 0x23
	OpGeF       byte = 0x24
	OpI2F       byte = 0x25

	// v3: string concatenation and str() conversions
	OpConcat byte = 0x26
	OpI2S    byte = 0x27
	OpF2S    byte = 0x28
	OpB2S    byte = 0x29

	// Portable primitive math
	OpMath byte = 0x32
	// Integer remainder
	OpModI byte = 0x33

	OpForRange byte = 0x2F
	OpForNext  byte = 0x31

	// v5: structured while loop markers
	OpWhileBegin byte = 0x34
	OpWhileTest  byte = 0x35
	OpLoopBack   byte = 0x36
)

// Math function IDs.
const (
	MathSinF     byte = 0
	MathCosF     byte = 1
	MathSqrtF    byte = 2
	MathAbsF     byte = 3
	MathFloorF   byte = 4
	MathPowF     byte = 5
	MathMinF     byte = 6
	MathMaxF     byte = 7
	MathClampF   byte = 8
	MathLerpF    byte = 9
	MathEaseF    byte = 10
	MathEaseInF  byte = 11
	MathEaseOutF byte = 12
	MathIntF     byte = 13
	MathAbsI     byte = 14
	MathMinI     byte = 15
	MathMaxI     byte = 16
	MathClampI   byte = 17
)

// RunStats records runtime execution statistics.
type RunStats struct {
	Instructions uint64
}

func minFloat(a, b float64) float64 {
	if math.IsNaN(a) {
		return a
	}
	if math.IsNaN(b) {
		return b
	}
	if a < b {
		return a
	}
	if b < a {
		return b
	}
	return a // equal operands, including signed zero, choose the first
}

func maxFloat(a, b float64) float64 {
	if math.IsNaN(a) {
		return a
	}
	if math.IsNaN(b) {
		return b
	}
	if a > b {
		return a
	}
	if b > a {
		return b
	}
	return a // equal operands, including signed zero, choose the first
}

func intFromFloat(x float64) int64 {
	if math.IsNaN(x) {
		return 0
	}
	const two63 = 9223372036854775808.0 // 0x1p63
	if x >= two63 {
		return math.MaxInt64
	}
	if x <= -two63 {
		return math.MinInt64
	}
	return int64(x)
}

// Run executes the loaded program once with the provided context and registry.
func (p *Program) Run(ctx *Context, reg *Registry, stats ...*RunStats) error {
	if reg != nil {
		reg.beginRun()
		defer reg.endRun()
	}
	stack := make([]Value, 0, p.maxStack)
	locals := make([]Value, len(p.locals))

	pos := 0
	code := p.code

	var statPtr *RunStats
	if len(stats) > 0 && stats[0] != nil {
		statPtr = stats[0]
	}

	pop := func() (Value, error) {
		if len(stack) == 0 {
			return None(), NewError(RUN_ERROR, "stack underflow")
		}
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return top, nil
	}

	push := func(v Value) {
		stack = append(stack, v)
	}

	jump := func(rel int32) error {
		target := int64(pos) + int64(rel)
		if target < 0 {
			return NewError(RUN_ERROR, "jump before the code start")
		}
		if target > int64(len(code)) {
			return NewError(RUN_ERROR, fmt.Sprintf("jump target %d is outside the code", target))
		}
		pos = int(target)
		return nil
	}

	readU8 := func() byte {
		v := code[pos]
		pos++
		return v
	}

	readU16 := func() uint16 {
		v := binary.BigEndian.Uint16(code[pos : pos+2])
		pos += 2
		return v
	}

	readU32 := func() uint32 {
		v := binary.BigEndian.Uint32(code[pos : pos+4])
		pos += 4
		return v
	}

	readI32 := func() int32 {
		return int32(readU32())
	}

	readI64 := func() int64 {
		v := binary.BigEndian.Uint64(code[pos : pos+8])
		pos += 8
		return int64(v)
	}

	readF64 := func() float64 {
		bits := binary.BigEndian.Uint64(code[pos : pos+8])
		pos += 8
		return math.Float64frombits(bits)
	}

	for {
		faultPC := pos
		if statPtr != nil {
			statPtr.Instructions++
		}
		op := readU8()

		step := func() error {
			switch op {
			case OpPushInt:
				push(Int(readI64()))
			case OpPushFloat:
				push(Float(readF64()))
			case OpPushStr:
				idx := readU32()
				push(String(p.consts[idx]))
			case OpPushBool:
				push(Bool(readU8() != 0))
			case OpLoadLocal:
				s := readU16()
				if locals[s].Kind == KindVoid {
					return NewError(RUN_ERROR, fmt.Sprintf("local slot %d read before assignment", s))
				}
				push(locals[s])
			case OpStoreLocal:
				s := readU16()
				v, err := pop()
				if err != nil {
					return err
				}
				locals[s] = v
			case OpLoadCtx:
				idx := readU16()
				name := p.context[idx]
				obj := ctx.Get(name)
				if obj == nil {
					return NewError(RUN_ERROR, fmt.Sprintf("context variable '%s' was not provided by the host for this run", name))
				}
				typeName, _ := reg.ContextType(name)
				push(Object(typeName, obj))
			case OpCallFn:
				idx := readU16()
				argc := readU8()
				imp := p.imports[idx]
				args := make([]Value, argc)
				for i := int(argc) - 1; i >= 0; i-- {
					v, err := pop()
					if err != nil {
						return err
					}
					args[i] = v
				}
				res, err := imp.fn.Fn(args)
				if err != nil {
					if e, ok := err.(*Error); ok {
						return e
					}
					return NewHostError(err.Error())
				}
				if res.Kind != KindVoid {
					push(res)
				}
			case OpCallMethod:
				idx := readU16()
				argc := readU8()
				imp := p.imports[idx]
				args := make([]Value, argc)
				for i := int(argc) - 1; i >= 0; i-- {
					v, err := pop()
					if err != nil {
						return err
					}
					args[i] = v
				}
				self, err := pop()
				if err != nil {
					return err
				}
				if self.Kind != KindObj || self.Type != imp.typeName {
					kindDesc := self.KindName()
					if self.Kind == KindObj {
						kindDesc = self.Type
					}
					return NewError(RUN_ERROR, fmt.Sprintf("method %s.%s called on a %s", imp.typeName, imp.methodName, kindDesc))
				}
				res, err := imp.method.Fn(self.Obj, args)
				if err != nil {
					if e, ok := err.(*Error); ok {
						return e
					}
					return NewHostError(err.Error())
				}
				if res.Kind != KindVoid {
					push(res)
				}
			case OpEq, OpNe:
				b, err := pop()
				if err != nil {
					return err
				}
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != b.Kind {
					return NewError(RUN_ERROR, fmt.Sprintf("cannot compare %s with %s", a.KindName(), b.KindName()))
				}
				var eq bool
				switch a.Kind {
				case KindVoid:
					eq = true
				case KindInt:
					eq = a.Int == b.Int
				case KindFloat:
					eq = a.Float == b.Float
				case KindStr:
					eq = a.Str == b.Str
				case KindBool:
					eq = a.Bool == b.Bool
				case KindObj:
					eq = a.Obj == b.Obj
				}
				if op == OpEq {
					push(Bool(eq))
				} else {
					push(Bool(!eq))
				}
			case OpAddI, OpSubI, OpMulI, OpDivI, OpModI, OpLtI, OpLeI, OpGtI, OpGeI:
				b, err := pop()
				if err != nil {
					return err
				}
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindInt || b.Kind != KindInt {
					return NewError(RUN_ERROR, fmt.Sprintf("int operator applied to %s and %s", a.KindName(), b.KindName()))
				}
				x, y := a.Int, b.Int
				switch op {
				case OpAddI:
					push(Int(x + y))
				case OpSubI:
					push(Int(x - y))
				case OpMulI:
					push(Int(x * y))
				case OpDivI:
					if y == 0 {
						return NewError(RUN_ERROR, "int division by zero")
					}
					if x == math.MinInt64 && y == -1 {
						push(Int(math.MinInt64))
					} else {
						push(Int(x / y))
					}
				case OpModI:
					if y == 0 {
						return NewError(RUN_ERROR, "int modulo by zero")
					}
					if x == math.MinInt64 && y == -1 {
						push(Int(0))
					} else {
						push(Int(x % y))
					}
				case OpLtI:
					push(Bool(x < y))
				case OpLeI:
					push(Bool(x <= y))
				case OpGtI:
					push(Bool(x > y))
				case OpGeI:
					push(Bool(x >= y))
				}
			case OpAddF, OpSubF, OpMulF, OpDivF, OpLtF, OpLeF, OpGtF, OpGeF:
				b, err := pop()
				if err != nil {
					return err
				}
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindFloat || b.Kind != KindFloat {
					return NewError(RUN_ERROR, fmt.Sprintf("float operator applied to %s and %s", a.KindName(), b.KindName()))
				}
				switch op {
				case OpAddF:
					push(Float(a.Float + b.Float))
				case OpSubF:
					push(Float(a.Float - b.Float))
				case OpMulF:
					push(Float(a.Float * b.Float))
				case OpDivF:
					push(Float(a.Float / b.Float))
				case OpLtF:
					push(Bool(a.Float < b.Float))
				case OpLeF:
					push(Bool(a.Float <= b.Float))
				case OpGtF:
					push(Bool(a.Float > b.Float))
				case OpGeF:
					push(Bool(a.Float >= b.Float))
				}
			case OpNegI:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindInt {
					return NewError(RUN_ERROR, fmt.Sprintf("int negation applied to %s", a.KindName()))
				}
				push(Int(-a.Int))
			case OpNegF:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindFloat {
					return NewError(RUN_ERROR, fmt.Sprintf("float negation applied to %s", a.KindName()))
				}
				push(Float(-a.Float))
			case OpI2F:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindInt {
					return NewError(RUN_ERROR, fmt.Sprintf("float() applied to %s", a.KindName()))
				}
				push(Float(float64(a.Int)))
			case OpConcat:
				b, err := pop()
				if err != nil {
					return err
				}
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindStr || b.Kind != KindStr {
					return NewError(RUN_ERROR, fmt.Sprintf("string + applied to %s and %s", a.KindName(), b.KindName()))
				}
				push(String(a.Str + b.Str))
			case OpI2S:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindInt {
					return NewError(RUN_ERROR, fmt.Sprintf("str(int) applied to %s", a.KindName()))
				}
				push(String(strconv.FormatInt(a.Int, 10)))
			case OpF2S:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindFloat {
					return NewError(RUN_ERROR, fmt.Sprintf("str(float) applied to %s", a.KindName()))
				}
				push(String(FormatG15(a.Float)))
			case OpB2S:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindBool {
					return NewError(RUN_ERROR, fmt.Sprintf("str(bool) applied to %s", a.KindName()))
				}
				if a.Bool {
					push(String("true"))
				} else {
					push(String("false"))
				}
			case OpMath:
				fn := readU8()
				switch fn {
				case MathSinF:
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(math.Sin(a.Float)))
				case MathCosF:
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(math.Cos(a.Float)))
				case MathSqrtF:
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(math.Sqrt(a.Float)))
				case MathAbsF:
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(math.Abs(a.Float)))
				case MathFloorF:
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(math.Floor(a.Float)))
				case MathPowF:
					b, err := pop()
					if err != nil {
						return err
					}
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(math.Pow(a.Float, b.Float)))
				case MathMinF:
					b, err := pop()
					if err != nil {
						return err
					}
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(minFloat(a.Float, b.Float)))
				case MathMaxF:
					b, err := pop()
					if err != nil {
						return err
					}
					a, err := pop()
					if err != nil {
						return err
					}
					push(Float(maxFloat(a.Float, b.Float)))
				case MathClampF:
					hi, err := pop()
					if err != nil {
						return err
					}
					lo, err := pop()
					if err != nil {
						return err
					}
					x, err := pop()
					if err != nil {
						return err
					}
					push(Float(minFloat(maxFloat(x.Float, lo.Float), hi.Float)))
				case MathLerpF:
					t, err := pop()
					if err != nil {
						return err
					}
					b, err := pop()
					if err != nil {
						return err
					}
					a, err := pop()
					if err != nil {
						return err
					}
					d := b.Float - a.Float
					push(Float(a.Float + d*t.Float))
				case MathEaseF:
					a, err := pop()
					if err != nil {
						return err
					}
					t := minFloat(maxFloat(a.Float, 0.0), 1.0)
					square := t * t
					twice := 2.0 * t
					push(Float(square * (3.0 - twice)))
				case MathEaseInF:
					a, err := pop()
					if err != nil {
						return err
					}
					t := minFloat(maxFloat(a.Float, 0.0), 1.0)
					square := t * t
					push(Float(square))
				case MathEaseOutF:
					a, err := pop()
					if err != nil {
						return err
					}
					t := minFloat(maxFloat(a.Float, 0.0), 1.0)
					d := 1.0 - t
					square := d * d
					push(Float(1.0 - square))
				case MathIntF:
					a, err := pop()
					if err != nil {
						return err
					}
					push(Int(intFromFloat(a.Float)))
				case MathAbsI:
					a, err := pop()
					if err != nil {
						return err
					}
					if a.Int < 0 {
						push(Int(-a.Int))
					} else {
						push(Int(a.Int))
					}
				case MathMinI:
					b, err := pop()
					if err != nil {
						return err
					}
					a, err := pop()
					if err != nil {
						return err
					}
					if a.Int < b.Int {
						push(Int(a.Int))
					} else {
						push(Int(b.Int))
					}
				case MathMaxI:
					b, err := pop()
					if err != nil {
						return err
					}
					a, err := pop()
					if err != nil {
						return err
					}
					if a.Int > b.Int {
						push(Int(a.Int))
					} else {
						push(Int(b.Int))
					}
				case MathClampI:
					hi, err := pop()
					if err != nil {
						return err
					}
					lo, err := pop()
					if err != nil {
						return err
					}
					x, err := pop()
					if err != nil {
						return err
					}
					lower := x.Int
					if lower < lo.Int {
						lower = lo.Int
					}
					if lower < hi.Int {
						push(Int(lower))
					} else {
						push(Int(hi.Int))
					}
				}
			case OpNot:
				a, err := pop()
				if err != nil {
					return err
				}
				if a.Kind != KindBool {
					return NewError(RUN_ERROR, fmt.Sprintf("! applied to a %s", a.KindName()))
				}
				push(Bool(!a.Bool))
			case OpJump:
				rel := readI32()
				return jump(rel)
			case OpJumpIfFalse, OpJumpIfTrue:
				rel := readI32()
				c, err := pop()
				if err != nil {
					return err
				}
				if c.Kind != KindBool {
					return NewError(RUN_ERROR, fmt.Sprintf("condition is a %s, not a bool", c.KindName()))
				}
				if c.Bool == (op == OpJumpIfTrue) {
					return jump(rel)
				}
			case OpForRange:
				counter := readU16()
				end := readU16()
				rel := readI32()
				finish, err := pop()
				if err != nil {
					return err
				}
				begin, err := pop()
				if err != nil {
					return err
				}
				locals[end] = finish
				locals[counter] = begin
				if locals[counter].Int >= locals[end].Int {
					return jump(rel)
				}
			case OpForNext:
				_ = readU16()
				rel := readI32()
				loop := p.loops[pos-7]
				locals[loop.counter].Int++
				if locals[loop.counter].Int < locals[loop.end].Int {
					return jump(rel)
				}
			case OpWhileBegin:
				// no-op marker
			case OpWhileTest:
				rel := readI32()
				condition, err := pop()
				if err != nil {
					return err
				}
				if !condition.Bool {
					return jump(rel)
				}
			case OpLoopBack:
				rel := readI32()
				return jump(rel)
			case OpDup:
				if len(stack) == 0 {
					return NewError(RUN_ERROR, "stack underflow")
				}
				push(stack[len(stack)-1])
			case OpPop:
				if _, err := pop(); err != nil {
					return err
				}
			case OpHalt:
				return nil
			default:
				return NewError(RUN_ERROR, fmt.Sprintf("unknown opcode %d", op))
			}
			return nil
		}

		err := step()
		if err != nil {
			var e *Error
			if ev, ok := err.(*Error); ok {
				e = ev
			} else {
				e = NewHostError(err.Error())
			}
			line := p.LineOf(faultPC)
			if line != nil {
				e.Locate(line.Line, line.Column)
			}
			return e
		}

		if op == OpHalt {
			return nil
		}
	}
}
