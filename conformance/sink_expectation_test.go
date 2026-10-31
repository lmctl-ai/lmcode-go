package conformance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmctl-ai/lmcode-go/conformance"
)

func TestSinkMetadataPreservesJSONEscapesOutsideTrace(t *testing.T) {
	payload := `{"text":"a\nb"}`
	frame := "LMCODE_RETURN_BEGIN\n" + payload + "\nLMCODE_RETURN_END\n"
	metadata, err := json.Marshal(map[string]string{"json": payload, "frame": frame})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "case.expect")
	if err := os.WriteFile(path, []byte("trace\r\nsink "+string(metadata)+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := conformance.ReadExpect(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 0 || got.Sink == nil || got.Sink.JSON != payload || got.Sink.Frame != frame {
		t.Fatalf("metadata became trace or lost escapes: %#v", got)
	}
}

func TestIncorrectSinkOrFrameOracleCannotPass(t *testing.T) {
	for _, field := range []string{"json", "frame"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"schema.json", "return-root.dslb"} {
				data, err := os.ReadFile(filepath.Join("vectors", name))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := os.ReadFile(filepath.Join("vectors", "return-root.expect"))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			var metadata map[string]string
			if err = json.Unmarshal([]byte(strings.TrimPrefix(lines[len(lines)-1], "sink ")), &metadata); err != nil {
				t.Fatal(err)
			}
			metadata[field] = "deliberately wrong oracle"
			changed, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			lines[len(lines)-1] = "sink " + string(changed)
			if err = os.WriteFile(filepath.Join(dir, "return-root.expect"), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = conformance.CheckVector(dir, "return-root"); err == nil {
				t.Fatal("incorrect " + field + " oracle was ignored")
			}
		})
	}
}
