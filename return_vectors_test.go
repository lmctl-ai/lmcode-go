package lmcode_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/conformance"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalReturnVectors(t *testing.T) {
	dir := "conformance/return-vectors"
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Vectors []struct {
			Name, Bytecode string
			Repeat         int
			Expect         struct {
				File string
				Exit int
			}
		}
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(filepath.Join(dir, "vectors.lock"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(lock)), "\n") {
		fields := strings.SplitN(line, "  ", 2)
		if len(fields) != 2 {
			t.Fatal(line)
		}
		data, e := os.ReadFile(filepath.Join(dir, fields[1]))
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != fields[0] {
			t.Fatal("stale canonical fixture", fields[1])
		}
	}
	for _, c := range manifest.Vectors {
		t.Run(c.Name, func(t *testing.T) {
			expected, e := os.ReadFile(filepath.Join(dir, c.Expect.File))
			if e != nil {
				t.Fatal(e)
			}
			out, code, e := conformance.ExecuteReturnFixture(dir, c.Bytecode, c.Repeat, false)
			if out != string(expected) || code != c.Expect.Exit {
				t.Fatalf("got (%d,%q,%v), want (%d,%q)", code, out, e, c.Expect.Exit, expected)
			}
			reasons := map[string]string{"entry-error": "entry failure", "unsafe-json-integer": "/amount: unsafe_integer", "scalar-field": "field assignment requires object or null", "invalid-utf8": ": utf8"}
			if reason := reasons[c.Name]; reason != "" {
				if e == nil || !strings.Contains(e.Error(), reason) {
					t.Fatal("wrong error", e)
				}
				if vm, ok := e.(*lmcode.Error); !ok || vm.Code != lmcode.RUN_HOST || vm.Line == 0 {
					t.Fatal("missing located RUN_HOST", e)
				}
			}
		})
	}
	out, code, err := conformance.ExecuteReturnFixture(dir, "entry-sink.dslb", 1, true)
	if out != "" || code != 4 || err != nil {
		t.Fatal("app failure emitted a frame", out, code, err)
	}
}
