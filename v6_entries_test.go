package lmcode_test

import (
	"encoding/binary"
	lmcode "github.com/lmctl-ai/lmcode-go"
	"strings"
	"testing"
)

var entrySchema = []byte("v6 entry unit schema")

func entryFixture(params uint16, ty uint32, hidden byte, iface string, code []byte) []byte {
	b := append([]byte("DSLB\x00\x06"), lmcode.SHA256(entrySchema)...)
	u16 := func(n uint16) { b = binary.BigEndian.AppendUint16(b, n) }
	u32 := func(n uint32) { b = binary.BigEndian.AppendUint32(b, n) }
	c := []string{"rule", "pair", iface, "int", "float"}
	u32(uint32(len(c)))
	for _, s := range c {
		b = append(b, 1)
		u32(uint32(len(s)))
		b = append(b, s...)
	}
	u16(0)
	u16(0)
	u16(0)
	u16(0)
	u32(1)
	b = append(b, 17)
	u32(0)
	u16(1)
	u32(0)
	u32(1)
	u32(2)
	u16(params)
	u16(1)
	u32(ty)
	b = append(b, hidden)
	u16(2)
	u32(uint32(len(code)))
	b = append(b, code...)
	u32(1)
	u32(0)
	u32(7)
	u16(9)
	return b
}
func TestV6EntryRefusals(t *testing.T) {
	cases := []struct {
		name   string
		params uint16
		ty     uint32
		hidden byte
		iface  string
		code   []byte
		want   string
	}{
		{"count", 0, 3, 0, "interface Step(int) -> int", []byte{4, 0, 0, 17}, "entry 'rule' declares 0 parameter(s); interface Step(int) -> int has 1"},
		{"type", 1, 4, 0, "interface Step(int) -> int", []byte{4, 0, 0, 17}, "entry 'rule' parameter 1 has type float; the interface expects int"},
		{"hidden", 1, 3, 1, "interface Step(int) -> int", []byte{4, 0, 0, 17}, "entry 'rule' parameter 1 is hidden"},
		{"value_halt", 1, 3, 0, "interface Step(int) -> int", []byte{17}, "entry 'rule': HALT leaves 0 values"},
		{"void_halt", 1, 3, 0, "interface Visit(int) -> void", []byte{4, 0, 0, 17}, "entry 'rule': HALT leaves 1 values"},
		{"jump", 1, 3, 0, "interface Step(int) -> int", []byte{12, 0, 0, 0, 99, 4, 0, 0, 17}, "entry 'rule': jump"},
		{"interface", 1, 3, 0, "not an interface", []byte{4, 0, 0, 17}, "entry 'rule': interface"},
		{"local", 1, 3, 0, "interface Step(int) -> int", []byte{4, 0, 1, 17}, "entry 'rule': local slot index 1 out of range"},
		{"constant", 1, 99, 0, "interface Step(int) -> int", []byte{4, 0, 0, 17}, "entry 'rule': local type: constant index 99"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, e := lmcode.Load(entryFixture(c.params, c.ty, c.hidden, c.iface, c.code), entrySchema, lmcode.NewRegistryWithoutStd())
			if e == nil || !strings.Contains(e.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, e)
			}
			if e.(*lmcode.Error).Code != lmcode.LOAD_BAD_BYTECODE {
				t.Fatal(e)
			}
		})
	}
}
func TestV5Refusal(t *testing.T) {
	b := append([]byte("DSLB\x00\x05"), lmcode.SHA256(entrySchema)...)
	b = append(b, make([]byte, 12)...)
	b = binary.BigEndian.AppendUint32(b, 1)
	b = append(b, 17, 0, 0, 0, 0)
	_, e := lmcode.Load(b, entrySchema, lmcode.NewRegistryWithoutStd())
	if e == nil || !strings.Contains(e.Error(), "unsupported bytecode version 5 (this runtime reads version 6)") {
		t.Fatal(e)
	}
}

func TestV6CallAPI(t *testing.T) {
	reg := lmcode.NewRegistryWithoutStd()
	p, e := lmcode.Load(entryFixture(1, 3, 0, "interface Step(int) -> int", []byte{4, 0, 0, 17}), entrySchema, reg)
	if e != nil {
		t.Fatal(e)
	}
	es := p.Entries()
	if len(es) != 1 || es[0].Name != "rule" || es[0].Annotation != "pair" || es[0].Iface != "interface Step(int) -> int" || es[0].Returns != "int" || es[0].Params[0] != "int" {
		t.Fatal(es)
	}
	es[0].Params[0] = "float"
	if p.Entries()[0].Params[0] != "int" {
		t.Fatal("mutable metadata")
	}
	ctx := lmcode.NewContext()
	stats := &lmcode.RunStats{}
	v, e := p.Call(0, ctx, reg, []lmcode.Value{lmcode.Int(42)}, stats)
	if e != nil || v.Int != 42 || stats.Instructions != 2 {
		t.Fatal(v, e, stats)
	}
	v, e = p.Call(0, ctx, reg, []lmcode.Value{lmcode.Bool(true)})
	if e != nil || v.Kind != lmcode.KindBool || !v.Bool {
		t.Fatal(v, e)
	}
	if e = p.Run(ctx, reg); e != nil {
		t.Fatal(e)
	}
	_, e = p.Call(0, ctx, reg, nil)
	if e == nil || e.(*lmcode.Error).Code != lmcode.RUN_HOST || e.Error() != "call entry 'rule' expects 1 argument(s); found 0" {
		t.Fatal(e)
	}
	_, e = p.Call(-1, ctx, reg, nil)
	if e == nil || e.(*lmcode.Error).Code != lmcode.RUN_HOST {
		t.Fatal(e)
	}
}
func TestV6CallLocatedErrorAndVoid(t *testing.T) {
	reg := lmcode.NewRegistryWithoutStd()
	code := []byte{4, 0, 0, 1}
	code = binary.BigEndian.AppendUint64(code, 0)
	code = append(code, 0x16, 17)
	p, e := lmcode.Load(entryFixture(1, 3, 0, "interface Step(int) -> int", code), entrySchema, reg)
	if e != nil {
		t.Fatal(e)
	}
	_, e = p.Call(0, lmcode.NewContext(), reg, []lmcode.Value{lmcode.Int(3)})
	if e == nil {
		t.Fatal("missing error")
	}
	fault := e.(*lmcode.Error)
	if fault.Code != lmcode.RUN_ERROR || fault.Line != 7 || fault.Column != 9 {
		t.Fatal(e)
	}
	p, e = lmcode.Load(entryFixture(1, 3, 0, "interface Visit(int) -> void", []byte{17}), entrySchema, reg)
	if e != nil {
		t.Fatal(e)
	}
	v, e := p.Call(0, lmcode.NewContext(), reg, []lmcode.Value{lmcode.Int(0)})
	if e != nil || v.Kind != lmcode.KindVoid {
		t.Fatal(v, e)
	}
}

// WP11: entry names are unique.
func TestV6DuplicateEntryName(t *testing.T) {
	b := entryFixture(1, 3, 0, "interface Step(int) -> int", []byte{4, 0, 0, 17})
	record := 12 + 2 + 2 + 5 + 2 + 4 + 4 + 4 + 10
	count := len(b) - record - 2
	d := append([]byte{}, b[:count]...)
	d = append(d, 0, 2)
	d = append(d, b[count+2:]...)
	d = append(d, b[count+2:]...)
	_, err := lmcode.Load(d, entrySchema, lmcode.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "duplicate entry name 'rule'") {
		t.Fatalf("duplicate entry name accepted: %v", err)
	}
}
