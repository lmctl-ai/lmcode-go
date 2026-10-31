package lmcode_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/lmctl-ai/lmcode-go"
)

var schemaBytes = []byte(`{"dsl.schema":1,"name":"t"}`)

const (
	tInt    = "method Trace.int(int) -> void"
	tFloat  = "method Trace.float(float) -> void"
	tStr    = "method Trace.str(string) -> void"
	thingID = "method Thing.id() -> int"
	twice   = "fn twice(int) -> int"
	boom    = "fn boom() -> void"
)

type thingObj struct {
	id int64
}

type traceObj struct {
	lines []string
}

type testHost struct {
	trace []string
	root  thingObj
	reg   *lmcode.Registry
}

func newTestHost() *testHost {
	h := &testHost{
		root: thingObj{id: 0},
		reg:  lmcode.NewRegistry(),
	}
	_ = h.reg.Context("t", "Trace")
	_ = h.reg.Context("root", "Thing")

	_ = h.reg.Method("Trace", "int", []string{"int"}, "void", func(self any, a []lmcode.Value) (lmcode.Value, error) {
		h.trace = append(h.trace, fmt.Sprintf("int %d", a[0].Int))
		return lmcode.None(), nil
	})
	_ = h.reg.Method("Trace", "float", []string{"float"}, "void", func(self any, a []lmcode.Value) (lmcode.Value, error) {
		h.trace = append(h.trace, fmt.Sprintf("float %s", lmcode.FormatG15(a[0].Float)))
		return lmcode.None(), nil
	})
	_ = h.reg.Method("Trace", "str", []string{"string"}, "void", func(self any, a []lmcode.Value) (lmcode.Value, error) {
		h.trace = append(h.trace, fmt.Sprintf("str %s", a[0].Str))
		return lmcode.None(), nil
	})
	_ = h.reg.Method("Thing", "id", nil, "int", func(self any, a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Int(self.(*thingObj).id), nil
	})
	_ = h.reg.Function("twice", []string{"int"}, "int", func(a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Int(a[0].Int * 2), nil
	})
	_ = h.reg.Function("boom", nil, "void", func(a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.None(), lmcode.NewHostError("boom from the host")
	})
	return h
}

func (h *testHost) run(bytecode []byte) ([]string, error) {
	p, err := lmcode.Load(bytecode, schemaBytes, h.reg)
	if err != nil {
		return nil, err
	}
	ctx := lmcode.NewContext()
	ctx.Set("t", &h.trace)
	ctx.Set("root", &h.root)
	h.trace = nil
	if err := p.Run(ctx, h.reg); err != nil {
		return nil, err
	}
	return h.trace, nil
}

type asm struct {
	consts   []string
	imports  []uint32
	contexts [][2]uint32
	locals   [][2]any // [typeNameIdx, hiddenByte]
	maxStack uint16
	code     []byte
	lines    [][3]uint32
}

func newAsm() *asm {
	return &asm{}
}

func (a *asm) k(s string) uint32 {
	for i, c := range a.consts {
		if c == s {
			return uint32(i)
		}
	}
	a.consts = append(a.consts, s)
	return uint32(len(a.consts) - 1)
}

func (a *asm) imp(canonical string) uint16 {
	c := a.k(canonical)
	for i, v := range a.imports {
		if v == c {
			return uint16(i)
		}
	}
	a.imports = append(a.imports, c)
	return uint16(len(a.imports) - 1)
}

func (a *asm) ctxIndex(name, typeName string) uint16 {
	nIdx := a.k(name)
	tIdx := a.k(typeName)
	pair := [2]uint32{nIdx, tIdx}
	for i, p := range a.contexts {
		if p == pair {
			return uint16(i)
		}
	}
	a.contexts = append(a.contexts, pair)
	return uint16(len(a.contexts) - 1)
}

func (a *asm) local(typeName string, hidden ...byte) *asm {
	h := byte(0)
	if len(hidden) > 0 {
		h = hidden[0]
	}
	a.locals = append(a.locals, [2]any{a.k(typeName), h})
	return a
}

func (a *asm) line(pc, ln uint32, col uint16) *asm {
	a.lines = append(a.lines, [3]uint32{pc, ln, uint32(col)})
	return a
}

func (a *asm) u8(v byte) *asm {
	a.code = append(a.code, v)
	return a
}

func (a *asm) u16(v uint16) *asm {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	a.code = append(a.code, buf...)
	return a
}

func (a *asm) u32(v uint32) *asm {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	a.code = append(a.code, buf...)
	return a
}

func (a *asm) i32(v int32) *asm {
	return a.u32(uint32(v))
}

func (a *asm) i64(v int64) *asm {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v))
	a.code = append(a.code, buf...)
	return a
}

func (a *asm) f64(v float64) *asm {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, math.Float64bits(v))
	a.code = append(a.code, buf...)
	return a
}

func (a *asm) pushInt(v int64) *asm {
	return a.u8(lmcode.OpPushInt).i64(v)
}

func (a *asm) pushFloat(v float64) *asm {
	return a.u8(lmcode.OpPushFloat).f64(v)
}

func (a *asm) pushStr(s string) *asm {
	return a.u8(lmcode.OpPushStr).u32(a.k(s))
}

func (a *asm) pushBool(b bool) *asm {
	val := byte(0)
	if b {
		val = 1
	}
	return a.u8(lmcode.OpPushBool).u8(val)
}

func (a *asm) ctx(n, typeName string) *asm {
	return a.u8(lmcode.OpLoadCtx).u16(a.ctxIndex(n, typeName))
}

func (a *asm) fn(canonical string, argc byte) *asm {
	return a.u8(lmcode.OpCallFn).u16(a.imp(canonical)).u8(argc)
}

func (a *asm) method(canonical string, argc byte) *asm {
	return a.u8(lmcode.OpCallMethod).u16(a.imp(canonical)).u8(argc)
}

func (a *asm) halt() *asm {
	return a.u8(lmcode.OpHalt)
}

func (a *asm) patch(at, target int) {
	rel := int32(target - (at + 4))
	binary.BigEndian.PutUint32(a.code[at:at+4], uint32(rel))
}

func (a *asm) file(schema []byte, version ...uint16) []byte {
	v := lmcode.Version
	if len(version) > 0 {
		v = version[0]
	}
	magic := "DSLB"
	return a.fileCustom(schema, v, magic)
}

func (a *asm) fileCustom(schema []byte, version uint16, magic string) []byte {
	var buf bytes.Buffer
	buf.WriteString(magic)
	_ = binary.Write(&buf, binary.BigEndian, version)
	h := sha256.Sum256(schema)
	buf.Write(h[:])

	_ = binary.Write(&buf, binary.BigEndian, uint32(len(a.consts)))
	for _, c := range a.consts {
		buf.WriteByte(1)
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(c)))
		buf.WriteString(c)
	}

	_ = binary.Write(&buf, binary.BigEndian, uint16(len(a.imports)))
	for _, imp := range a.imports {
		_ = binary.Write(&buf, binary.BigEndian, imp)
	}

	_ = binary.Write(&buf, binary.BigEndian, uint16(len(a.contexts)))
	for _, ctx := range a.contexts {
		_ = binary.Write(&buf, binary.BigEndian, ctx[0])
		_ = binary.Write(&buf, binary.BigEndian, ctx[1])
	}

	_ = binary.Write(&buf, binary.BigEndian, uint16(len(a.locals)))
	for _, l := range a.locals {
		_ = binary.Write(&buf, binary.BigEndian, l[0].(uint32))
		buf.WriteByte(l[1].(byte))
	}

	_ = binary.Write(&buf, binary.BigEndian, a.maxStack)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(a.code)))
	buf.Write(a.code)

	_ = binary.Write(&buf, binary.BigEndian, uint32(len(a.lines)))
	for _, l := range a.lines {
		_ = binary.Write(&buf, binary.BigEndian, l[0])
		_ = binary.Write(&buf, binary.BigEndian, l[1])
		_ = binary.Write(&buf, binary.BigEndian, uint16(l[2]))
	}

	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // v6 entries
	return buf.Bytes()
}

func assertTrace(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d lines, want %d\ngot:  %v\nwant: %v", label, len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s line %d: got %q, want %q", label, i, got[i], want[i])
		}
	}
}

func assertError(t *testing.T, label string, err error, needle string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error containing %q, got nil", label, needle)
	}
	if !strings.Contains(err.Error(), needle) {
		t.Fatalf("%s: got error %q, want %q", label, err.Error(), needle)
	}
}

func TestValidPrograms(t *testing.T) {
	h := newTestHost()

	// Arithmetic, concat, method calls
	{
		a := newAsm()
		a.ctx("t", "Trace").pushInt(1).pushInt(2).u8(lmcode.OpAddI).method(tInt, 1).
			ctx("t", "Trace").pushStr("a").pushStr("b").u8(lmcode.OpConcat).method(tStr, 1).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "arithmetic and concat", got, []string{"int 3", "str ab"})
	}

	// Host function and locals
	{
		a := newAsm()
		a.local("int").pushInt(21).fn(twice, 1).u8(lmcode.OpStoreLocal).u16(0).
			ctx("t", "Trace").u8(lmcode.OpLoadLocal).u16(0).method(tInt, 1).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "host function and locals", got, []string{"int 42"})
	}

	// Forward jumps select a branch
	{
		a := newAsm()
		a.pushBool(true).u8(lmcode.OpJumpIfFalse).i32(21).
			ctx("t", "Trace").pushInt(1).method(tInt, 1).
			u8(lmcode.OpJump).i32(16).
			ctx("t", "Trace").pushInt(2).method(tInt, 1).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "forward jumps", got, []string{"int 1"})
	}

	// Context object, floats, conversions, equality
	{
		a := newAsm()
		a.ctx("t", "Trace").ctx("root", "Thing").method(thingID, 0).method(tInt, 1).
			ctx("t", "Trace").pushFloat(2.5).u8(lmcode.OpF2S).method(tStr, 1).
			ctx("t", "Trace").pushInt(7).pushInt(2).u8(lmcode.OpDivI).u8(lmcode.OpI2F).pushFloat(0.5).u8(lmcode.OpAddF).u8(lmcode.OpF2S).method(tStr, 1).
			ctx("t", "Trace").pushInt(3).pushInt(3).u8(lmcode.OpEq).u8(lmcode.OpB2S).method(tStr, 1).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "conversions and equality", got, []string{"int 0", "str 2.5", "str 3.5", "str true"})
	}

	// Empty program loads
	{
		a := newAsm().halt()
		p, err := lmcode.Load(a.file(schemaBytes), schemaBytes, h.reg)
		if err != nil {
			t.Fatal(err)
		}
		if p.ConstantCount() != 0 || p.CodeSize() != 1 || p.ImportCount() != 0 || p.LocalCount() != 0 || len(p.Lines()) != 0 {
			t.Fatalf("unexpected counts in empty program: const=%d, code=%d, import=%d, local=%d, lines=%d",
				p.ConstantCount(), p.CodeSize(), p.ImportCount(), p.LocalCount(), len(p.Lines()))
		}
	}
}

func TestMathFunctions(t *testing.T) {
	h := newTestHost()
	a := newAsm()

	fs := func(x float64, fn byte) {
		a.ctx("t", "Trace").pushFloat(x).u8(lmcode.OpMath).u8(fn).u8(lmcode.OpF2S).method(tStr, 1)
	}
	fb := func(x, y float64, fn byte) {
		a.ctx("t", "Trace").pushFloat(x).pushFloat(y).u8(lmcode.OpMath).u8(fn).u8(lmcode.OpF2S).method(tStr, 1)
	}
	fc := func(x, lo, hi float64, fn byte) {
		a.ctx("t", "Trace").pushFloat(x).pushFloat(lo).pushFloat(hi).u8(lmcode.OpMath).u8(fn).u8(lmcode.OpF2S).method(tStr, 1)
	}
	is := func(x int64, fn byte) {
		a.ctx("t", "Trace").pushInt(x).u8(lmcode.OpMath).u8(fn).method(tInt, 1)
	}
	ib := func(x, y int64, fn byte) {
		a.ctx("t", "Trace").pushInt(x).pushInt(y).u8(lmcode.OpMath).u8(fn).method(tInt, 1)
	}
	ic := func(x, lo, hi int64, fn byte) {
		a.ctx("t", "Trace").pushInt(x).pushInt(lo).pushInt(hi).u8(lmcode.OpMath).u8(fn).method(tInt, 1)
	}

	fs(0.0, lmcode.MathSinF)
	fs(0.0, lmcode.MathCosF)
	fs(4.0, lmcode.MathSqrtF)
	fs(-2.5, lmcode.MathAbsF)
	fs(1.8, lmcode.MathFloorF)
	fb(2.0, 3.0, lmcode.MathPowF)
	fb(2.0, 3.0, lmcode.MathMinF)
	fb(3.0, 2.0, lmcode.MathMinF)
	fb(2.0, 3.0, lmcode.MathMaxF)
	fb(3.0, 2.0, lmcode.MathMaxF)
	fc(5.0, 0.0, 3.0, lmcode.MathClampF)
	fc(2.0, 5.0, 0.0, lmcode.MathClampF)
	a.ctx("t", "Trace").pushFloat(0.0).pushFloat(10.0).pushFloat(0.25).u8(lmcode.OpMath).u8(lmcode.MathLerpF).u8(lmcode.OpF2S).method(tStr, 1)
	fs(0.5, lmcode.MathEaseF)
	fs(0.5, lmcode.MathEaseInF)
	fs(0.5, lmcode.MathEaseOutF)
	fs(math.NaN(), lmcode.MathEaseF)
	a.ctx("t", "Trace").pushFloat(-2.8).u8(lmcode.OpMath).u8(lmcode.MathIntF).method(tInt, 1)
	is(math.MinInt64, lmcode.MathAbsI)
	ib(2, 3, lmcode.MathMinI)
	ib(2, 3, lmcode.MathMaxI)
	ic(5, 0, 3, lmcode.MathClampI)
	ic(2, 5, 0, lmcode.MathClampI)
	a.ctx("t", "Trace").pushFloat(0.0).pushFloat(math.Copysign(0, -1)).u8(lmcode.OpMath).u8(lmcode.MathMinF).u8(lmcode.OpF2S).method(tStr, 1)
	a.ctx("t", "Trace").pushFloat(math.Copysign(0, -1)).pushFloat(0.0).u8(lmcode.OpMath).u8(lmcode.MathMaxF).u8(lmcode.OpF2S).method(tStr, 1).halt()

	got, err := h.run(a.file(schemaBytes))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"str 0", "str 1", "str 2", "str 2.5", "str 1", "str 8", "str 2", "str 2", "str 3", "str 3", "str 3", "str 0",
		"str 2.5", "str 0.5", "str 0.25", "str 0.75", "str nan", "int -2", "int -9223372036854775808",
		"int 2", "int 3", "int 3", "int 0", "str 0", "str -0",
	}
	assertTrace(t, "MATH functions and edge semantics", got, want)
}

func TestStructuredLoops(t *testing.T) {
	h := newTestHost()

	// While loop runs 0..2
	{
		a := newAsm()
		a.local("int").pushInt(0).u8(lmcode.OpStoreLocal).u16(0)
		head := len(a.code)
		a.u8(lmcode.OpWhileBegin).u8(lmcode.OpLoadLocal).u16(0).pushInt(3).u8(lmcode.OpLtI).u8(lmcode.OpWhileTest)
		exit := len(a.code)
		a.i32(0).ctx("t", "Trace").u8(lmcode.OpLoadLocal).u16(0).method(tInt, 1).
			u8(lmcode.OpLoadLocal).u16(0).pushInt(1).u8(lmcode.OpAddI).u8(lmcode.OpStoreLocal).u16(0).
			u8(lmcode.OpLoopBack)
		back := len(a.code)
		a.i32(0)
		end := len(a.code)
		a.patch(exit, end)
		a.patch(back, head)
		a.halt()

		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "while loop", got, []string{"int 0", "int 1", "int 2"})
	}

	// While loop zero iterations
	{
		a := newAsm()
		a.u8(lmcode.OpWhileBegin).pushBool(false).u8(lmcode.OpWhileTest).i32(5).u8(lmcode.OpLoopBack).i32(-13).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "while zero iter", got, nil)
	}

	// Range loop 0..2
	{
		a := newAsm()
		a.local("int", 1).local("int", 1).pushInt(0).pushInt(3).
			u8(lmcode.OpForRange).u16(0).u16(1).i32(17).
			ctx("t", "Trace").u8(lmcode.OpLoadLocal).u16(0).method(tInt, 1).
			u8(lmcode.OpForNext).u16(1).i32(-17).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "range loop 0..2", got, []string{"int 0", "int 1", "int 2"})
	}

	// Empty range skips body
	{
		a := newAsm()
		a.local("int", 1).local("int", 1).pushInt(2).pushInt(2).
			u8(lmcode.OpForRange).u16(0).u16(1).i32(23).
			ctx("t", "Trace").pushInt(99).method(tInt, 1).
			u8(lmcode.OpForNext).u16(1).i32(-23).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "empty range", got, nil)
	}

	// Range ending at MaxInt64 does not overflow
	{
		a := newAsm()
		a.local("int", 1).local("int", 1).pushInt(math.MaxInt64-1).pushInt(math.MaxInt64).
			u8(lmcode.OpForRange).u16(0).u16(1).i32(17).
			ctx("t", "Trace").u8(lmcode.OpLoadLocal).u16(0).method(tInt, 1).
			u8(lmcode.OpForNext).u16(1).i32(-17).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "range ending at MaxInt64", got, []string{"int 9223372036854775806"})
	}

	// Range starting at MinInt64 does not overflow
	{
		a := newAsm()
		a.local("int", 1).local("int", 1).pushInt(math.MinInt64).pushInt(math.MinInt64+2).
			u8(lmcode.OpForRange).u16(0).u16(1).i32(17).
			ctx("t", "Trace").u8(lmcode.OpLoadLocal).u16(0).method(tInt, 1).
			u8(lmcode.OpForNext).u16(1).i32(-17).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		assertTrace(t, "range starting at MinInt64", got, []string{"int -9223372036854775808", "int -9223372036854775807"})
	}
}

func TestInt64WraparoundAndRemainder(t *testing.T) {
	h := newTestHost()
	max := int64(math.MaxInt64)
	min := int64(math.MinInt64)

	a := newAsm()
	a.ctx("t", "Trace").pushInt(max).method(tInt, 1).
		ctx("t", "Trace").pushInt(min).method(tInt, 1).
		ctx("t", "Trace").pushInt(max).pushInt(1).u8(lmcode.OpAddI).method(tInt, 1).
		ctx("t", "Trace").pushInt(min).u8(lmcode.OpNegI).method(tInt, 1).
		ctx("t", "Trace").pushInt(min).pushInt(-1).u8(lmcode.OpDivI).method(tInt, 1).
		ctx("t", "Trace").pushInt(min).pushInt(-1).u8(lmcode.OpModI).method(tInt, 1).
		ctx("t", "Trace").pushInt(min).pushInt(1).u8(lmcode.OpSubI).method(tInt, 1).
		ctx("t", "Trace").pushInt(max).pushInt(max).u8(lmcode.OpMulI).method(tInt, 1).
		ctx("t", "Trace").pushInt(7).pushInt(3).u8(lmcode.OpModI).method(tInt, 1).
		ctx("t", "Trace").pushInt(-7).pushInt(3).u8(lmcode.OpModI).method(tInt, 1).
		ctx("t", "Trace").pushInt(7).pushInt(-3).u8(lmcode.OpModI).method(tInt, 1).
		ctx("t", "Trace").pushInt(-7).pushInt(2).u8(lmcode.OpDivI).method(tInt, 1).halt()

	got, err := h.run(a.file(schemaBytes))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"int 9223372036854775807",
		"int -9223372036854775808",
		"int -9223372036854775808",
		"int -9223372036854775808",
		"int -9223372036854775808",
		"int 0",
		"int 9223372036854775807",
		"int 1",
		"int 1",
		"int -1",
		"int 1",
		"int -3",
	}
	assertTrace(t, "int64 wraparound and remainder", got, want)
}

func TestLoaderRejections(t *testing.T) {
	h := newTestHost()

	load := func(b []byte, s ...[]byte) error {
		schema := schemaBytes
		if len(s) > 0 {
			schema = s[0]
		}
		_, err := lmcode.Load(b, schema, h.reg)
		return err
	}

	assertError(t, "bad magic", load(newAsm().halt().fileCustom(schemaBytes, 5, "XXXX")), "bad magic")
	assertError(t, "version 3 refused", load(newAsm().halt().file(schemaBytes, 3)), "unsupported bytecode version 3")
	assertError(t, "version 4 refused", load(newAsm().halt().file(schemaBytes, 4)), "unsupported bytecode version 4")
	assertError(t, "schema mismatch", load(newAsm().halt().file(schemaBytes), append(schemaBytes, ' ')), "schema mismatch")

	// Truncation at every prefix
	validFile := newAsm().ctx("t", "Trace").halt().file(schemaBytes)
	for i := 0; i < len(validFile); i++ {
		err := load(validFile[:i])
		if err == nil {
			t.Fatalf("truncation at offset %d did not fail", i)
		}
	}

	assertError(t, "trailing bytes", load(append(validFile, 'x')), "trailing byte(s)")

	// Unknown constant tag
	{
		f := newAsm().pushStr("x").halt().file(schemaBytes)
		f[4+2+32+4] = 7
		assertError(t, "unknown constant tag", load(f), "constant 0 has unknown tag 7")
	}

	// Constant index out of range
	assertError(t, "constant index out of range", load(newAsm().u8(lmcode.OpPushStr).u32(9).halt().file(schemaBytes)), "constant index 9 out of range")
	// Local slot out of range
	assertError(t, "local slot out of range", load(newAsm().u8(lmcode.OpLoadLocal).u16(0).halt().file(schemaBytes)), "local slot index 0 out of range")
	// Context index out of range
	assertError(t, "context index out of range", load(newAsm().u8(lmcode.OpLoadCtx).u16(0).halt().file(schemaBytes)), "context index 0 out of range")

	// Undeclared context
	assertError(t, "undeclared context", load(newAsm().ctx("nope", "Trace").halt().file(schemaBytes)), "context variable 'nope' is not declared")
	// Context type differs
	assertError(t, "context type differs", load(newAsm().ctx("t", "Thing").halt().file(schemaBytes)), "declared as Thing by the bytecode, the host provides Trace")

	// Unbound function
	assertError(t, "unbound function", load(newAsm().fn("fn missing() -> int", 0).halt().file(schemaBytes)), "import 'fn missing() -> int' is not provided by the host")
	// Import with other types names host signature
	assertError(t, "import with other types", load(newAsm().pushFloat(1.0).fn("fn twice(float) -> int", 1).halt().file(schemaBytes)), "(the host declares fn twice(int) -> int)")

	// Arity mismatch
	assertError(t, "CALL_FN arity", load(newAsm().pushInt(1).pushInt(2).fn(twice, 2).halt().file(schemaBytes)), "passes 2 argument(s), the import takes 1")
	// Method import as fn
	assertError(t, "CALL_FN on method", load(newAsm().ctx("root", "Thing").fn(thingID, 0).halt().file(schemaBytes)), "which is a method")
	// Fn import as method
	assertError(t, "CALL_METHOD on fn", load(newAsm().pushInt(1).method(twice, 1).halt().file(schemaBytes)), "which is a function")

	// Backward jump
	assertError(t, "backward jump", load(newAsm().u8(lmcode.OpJump).i32(-5).halt().file(schemaBytes)), "goes backwards")
	// Jump into operand
	assertError(t, "jump into operand", load(newAsm().u8(lmcode.OpJump).i32(1).halt().file(schemaBytes)), "not an instruction start")
	// Stray LOOP_BACK
	assertError(t, "stray LOOP_BACK", load(newAsm().u8(lmcode.OpLoopBack).i32(-5).halt().file(schemaBytes)), "has no matching WHILE_BEGIN")
	// Unpaired WHILE_BEGIN
	assertError(t, "unpaired WHILE_BEGIN", load(newAsm().u8(lmcode.OpWhileBegin).file(schemaBytes)), "unclosed WHILE_BEGIN")

	// WP11: a hidden local is compiler-owned even when no loop uses it
	assertError(t, "hidden store", load(newAsm().local("int", 1).pushInt(0).u8(lmcode.OpStoreLocal).u16(0).halt().file(schemaBytes)), "writes compiler-owned loop local 0")

	// Read-only range slot
	assertError(t, "read-only range slot", load(newAsm().local("int", 1).local("int", 1).pushInt(0).pushInt(1).
		u8(lmcode.OpForRange).u16(0).u16(1).i32(10).u8(lmcode.OpStoreLocal).u16(0).u8(lmcode.OpForNext).u16(1).i32(-10).halt().file(schemaBytes)), "read-only range slot")

	// Missing HALT
	assertError(t, "missing HALT", load(newAsm().pushInt(1).file(schemaBytes)), "does not end with HALT")
	// Unknown opcode
	assertError(t, "unknown opcode", load(newAsm().u8(0x7f).halt().file(schemaBytes)), "unknown opcode 127")
	// Unknown MATH function
	assertError(t, "unknown MATH function", load(newAsm().u8(lmcode.OpMath).u8(18).halt().file(schemaBytes)), "unknown MATH function 18")
	// Bad bool operand
	assertError(t, "bad bool operand", load(newAsm().u8(lmcode.OpPushBool).u8(2).halt().file(schemaBytes)), "PUSH_BOOL operand must be 0 or 1")
	// Bad hidden flag
	assertError(t, "bad hidden flag", load(newAsm().local("int", 2).halt().file(schemaBytes)), "hidden flag must be 0 or 1")
	// Unsorted line table
	assertError(t, "unsorted line table", load(newAsm().line(5, 1, 1).line(2, 2, 1).pushInt(1).u8(lmcode.OpPop).halt().file(schemaBytes)), "line table is not sorted")
	// Line beyond code
	assertError(t, "line beyond code", load(newAsm().line(99, 1, 1).halt().file(schemaBytes)), "beyond the code")
}

func TestRuntimeChecks(t *testing.T) {
	h := newTestHost()

	run := func(a *asm) error {
		_, err := h.run(a.file(schemaBytes))
		return err
	}

	assertError(t, "int div by zero", run(newAsm().pushInt(1).pushInt(0).u8(lmcode.OpDivI).halt()), "int division by zero")
	assertError(t, "int mod by zero", run(newAsm().pushInt(1).pushInt(0).u8(lmcode.OpModI).halt()), "int modulo by zero")
	assertError(t, "mixed-kind equality", run(newAsm().pushInt(1).pushStr("x").u8(lmcode.OpEq).halt()), "cannot compare int with string")
	assertError(t, "float op on int", run(newAsm().pushInt(1).pushFloat(1.0).u8(lmcode.OpAddF).halt()), "float operator applied to int and float")
	assertError(t, "local read before store", run(newAsm().local("int").u8(lmcode.OpLoadLocal).u16(0).halt()), "read before assignment")
	assertError(t, "non-bool condition", run(newAsm().pushInt(1).u8(lmcode.OpJumpIfFalse).i32(0).halt()), "condition is a int, not a bool")
	assertError(t, "stack underflow", run(newAsm().u8(lmcode.OpPop).halt()), "stack underflow")
	assertError(t, "host error", run(newAsm().fn(boom, 0).halt()), "boom from the host")
}

func TestRuntimeErrorCodes(t *testing.T) {
	h := newTestHost()

	_, err := h.run(newAsm().pushInt(1).pushStr("x").u8(lmcode.OpEq).halt().file(schemaBytes))
	lmErr, ok := err.(*lmcode.Error)
	if !ok {
		t.Fatalf("expected *lmcode.Error for VM type error, got %T", err)
	}
	if lmErr.Code != lmcode.RUN_ERROR {
		t.Fatalf("VM type error code = %s, want %s", lmErr.Code, lmcode.RUN_ERROR)
	}

	_, err = h.run(newAsm().fn(boom, 0).halt().file(schemaBytes))
	lmErr, ok = err.(*lmcode.Error)
	if !ok {
		t.Fatalf("expected *lmcode.Error for host callback error, got %T", err)
	}
	if lmErr.Code != lmcode.RUN_HOST {
		t.Fatalf("host callback error code = %s, want %s", lmErr.Code, lmcode.RUN_HOST)
	}
}

func TestSourceLineLocation(t *testing.T) {
	h := newTestHost()

	// Error located at line 9:7
	a := newAsm().line(0, 1, 1).pushInt(1).pushInt(0).line(18, 9, 7).u8(lmcode.OpDivI).line(19, 99, 1).halt()
	_, err := h.run(a.file(schemaBytes))
	if err == nil {
		t.Fatal("expected error")
	}
	lmErr, ok := err.(*lmcode.Error)
	if !ok {
		t.Fatalf("expected *lmcode.Error, got %T", err)
	}
	if lmErr.Code != lmcode.RUN_ERROR || !lmErr.HasLocation || lmErr.Line != 9 || lmErr.Column != 7 {
		t.Fatalf("unexpected location: code=%s, loc=%v, line=%d, col=%d", lmErr.Code, lmErr.HasLocation, lmErr.Line, lmErr.Column)
	}
	if !strings.HasPrefix(lmErr.Message, "dsl line 9:7: int division by zero") {
		t.Fatalf("message mismatch: %s", lmErr.Message)
	}

	// Host error located at call opcode
	a2 := newAsm().line(0, 4, 2).fn(boom, 0).halt()
	_, err2 := h.run(a2.file(schemaBytes))
	lmErr2 := err2.(*lmcode.Error)
	if lmErr2.Code != lmcode.RUN_HOST || !lmErr2.HasLocation || lmErr2.Line != 4 || lmErr2.Column != 2 {
		t.Fatalf("unexpected location for host error: code=%s, line=%d, col=%d", lmErr2.Code, lmErr2.Line, lmErr2.Column)
	}

	// Unlocated error when line table empty
	a3 := newAsm().pushInt(1).pushInt(0).u8(lmcode.OpDivI).halt()
	_, err3 := h.run(a3.file(schemaBytes))
	lmErr3 := err3.(*lmcode.Error)
	if lmErr3.HasLocation {
		t.Fatalf("expected unlocated error with empty line table")
	}

	// Locate helper method
	e := lmcode.NewError(lmcode.RUN_ERROR, "x")
	e.Locate(6, 0)
	if !e.HasLocation || e.HasColumn || e.Message != "dsl line 6: x" {
		t.Fatalf("Locate(6, 0) failed: %v", e)
	}
	empty := lmcode.NewError(lmcode.RUN_ERROR, "x")
	empty.Locate(0, 0)
	if empty.HasLocation {
		t.Fatalf("Locate(0, 0) should not set location")
	}
}

func TestJSONResultHelpers(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	full := lmcode.NewError(lmcode.RUN_ERROR, "bad")
	full.Locate(4, 2)
	lineOnly := lmcode.NewError(lmcode.RUN_HOST, "host")
	lineOnly.Locate(5, 0)
	plain := lmcode.NewError(lmcode.APP_FAIL, "app")

	c1 := lmcode.FailureFrame(os.Stdout, "run", full)
	c2 := lmcode.FailureFrame(os.Stdout, "run", lineOnly)
	c3 := lmcode.FailureFrame(os.Stdout, "app", plain)

	_ = w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	if c1 != 3 || c2 != 3 || c3 != 4 {
		t.Fatalf("unexpected exit codes: %d, %d, %d", c1, c2, c3)
	}

	want := "LMCODE_RETURN_BEGIN\n{\"ok\":false,\"phase\":\"run\",\"code\":\"RUN_ERROR\",\"message\":\"dsl line 4:2: bad\",\"line\":4,\"column\":2}\nLMCODE_RETURN_END\n" +
		"LMCODE_RETURN_BEGIN\n{\"ok\":false,\"phase\":\"run\",\"code\":\"RUN_HOST\",\"message\":\"dsl line 5: host\",\"line\":5}\nLMCODE_RETURN_END\n" +
		"LMCODE_RETURN_BEGIN\n{\"ok\":false,\"phase\":\"app\",\"code\":\"APP_FAIL\",\"message\":\"app\"}\nLMCODE_RETURN_END\n"

	if string(out) != want {
		t.Fatalf("JSON output mismatch:\ngot:  %q\nwant: %q", string(out), want)
	}

	if lmcode.JSONEscape("a\"b\n") != `"a\"b\n"` {
		t.Fatalf("JSONEscape failed: %s", lmcode.JSONEscape("a\"b\n"))
	}
}

func TestNoResourceLimits(t *testing.T) {
	h := newTestHost()

	// 5000 stack operands
	{
		a := newAsm()
		a.maxStack = 1
		for i := 0; i < 5000; i++ {
			a.pushInt(1)
		}
		for i := 0; i < 4999; i++ {
			a.u8(lmcode.OpAddI)
		}
		a.ctx("t", "Trace").u8(lmcode.OpDup).u8(lmcode.OpPop).halt()
		p, err := lmcode.Load(a.file(schemaBytes), schemaBytes, h.reg)
		if err != nil {
			t.Fatal(err)
		}
		ctx := lmcode.NewContext()
		ctx.Set("t", &h.trace)
		h.trace = nil
		if err := p.Run(ctx, h.reg); err != nil {
			t.Fatal(err)
		}
	}

	// Large string concatenation
	{
		a := newAsm()
		a.ctx("t", "Trace").pushStr(strings.Repeat("x", 65536))
		for i := 0; i < 9; i++ {
			a.u8(lmcode.OpDup).u8(lmcode.OpConcat)
		}
		a.method(tStr, 1).halt()
		got, err := h.run(a.file(schemaBytes))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || len(got[0]) != 4+65536*512 {
			t.Fatalf("expected 32 MiB string, got len %d", len(got[0]))
		}
	}
}
