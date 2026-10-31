package conformance

import (
	"bytes"
	"github.com/lmctl-ai/lmcode-go"
	"os"
	"path/filepath"
)

// Canonical C++ host policy, not a VM auto-reporter: frame only after the whole
// body/entry/application transaction succeeds. HALT values stay independent.
func ExecuteReturnFixture(dir, file string, repeat int, appFail bool) (string, int, error) {
	schema, err := os.ReadFile(filepath.Join(dir, "schema.json"))
	if err != nil {
		return "", 3, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", 3, err
	}
	r := lmcode.NewRegistry()
	sink := &lmcode.ResultSink{}
	if err = r.SetResultSink(sink); err != nil {
		return "", 3, err
	}
	err = r.Function("entry_fail", nil, "void", func([]lmcode.Value) (lmcode.Value, error) { return lmcode.None(), lmcode.NewHostError("entry failure") })
	if err != nil {
		return "", 3, err
	}
	err = r.Function("expect_result", []string{"string"}, "void", func(a []lmcode.Value) (lmcode.Value, error) {
		if sink.JSON() != a[0].Str {
			return lmcode.None(), lmcode.NewHostError("return overload payload mismatch")
		}
		return lmcode.None(), nil
	})
	if err != nil {
		return "", 3, err
	}
	program, err := lmcode.Load(raw, schema, r)
	if err != nil {
		return "", 3, err
	}
	context := lmcode.NewContext()
	sink.Reset()
	err = program.Run(context, r)
	if err == nil {
		for n := 0; n < repeat; n++ {
			for i := range program.Entries() {
				var halt lmcode.Value
				halt, err = program.Call(i, context, r, nil)
				if err != nil {
					break
				}
				if halt.Kind != lmcode.KindInt || halt.Int != 7 {
					err = lmcode.NewHostError("entry HALT result changed")
					break
				}
			}
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		return "", 3, err
	}
	if appFail {
		return "", 4, nil
	}
	var out bytes.Buffer
	if err = lmcode.WriteResultFrame(&out, sink); err != nil {
		return "", 3, err
	}
	return out.String(), 0, nil
}
