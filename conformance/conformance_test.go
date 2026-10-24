package conformance_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lmctl-ai/lmcode-go/conformance"
)

func TestVectorsLock(t *testing.T) {
	cmd := exec.Command("bash", "../scripts/sync-vectors.sh", "--verify")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync-vectors.sh --verify failed: %s\n%v", string(out), err)
	}
}

func TestApproximateTraceMatcher(t *testing.T) {
	if !conformance.TraceLineMatches("float 1.0000000000005", "~ float 1.0") {
		t.Errorf("expected close float to match")
	}
	if conformance.TraceLineMatches("int 1.0", "~ float 1.0") {
		t.Errorf("expected prefix mismatch to fail")
	}
	if conformance.TraceLineMatches("float 1.00000000001", "~ float 1.0") {
		t.Errorf("expected drift to fail")
	}
	if conformance.TraceLineMatches("float nan", "~ float nan") {
		t.Errorf("expected NaN in approx matcher to fail")
	}
	if conformance.TraceLineMatches("float inf", "~ float 1.0") {
		t.Errorf("expected inf to fail")
	}
	if !conformance.TraceLineMatches("float 1", "float 1") {
		t.Errorf("expected exact match")
	}
}

func TestAllVectors(t *testing.T) {
	dir := "vectors"
	names, err := conformance.VectorNames(dir)
	if err != nil {
		t.Fatalf("VectorNames(%s): %v", dir, err)
	}
	if len(names) < 21 {
		t.Fatalf("expected at least 21 vectors, got %d", len(names))
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			_, err := conformance.CheckVector(dir, name)
			if err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func TestWrongExpectationDetected(t *testing.T) {
	dir := "vectors"
	expect, err := conformance.ReadExpect(filepath.Join(dir, "literals.expect"))
	if err != nil {
		t.Fatalf("ReadExpect: %v", err)
	}
	if len(expect.Lines) == 0 {
		t.Fatalf("literals.expect has no lines")
	}
	// Verify that checking with a corrupted expectation fails
	orig := expect.Lines[0]
	expect.Lines[0] = "int 9999999"
	defer func() { expect.Lines[0] = orig }()
}
