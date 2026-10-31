#!/usr/bin/env python3
"""Compile semantic dry-mode mutants in isolated copies; build failures never count as catches."""
import pathlib
import shutil
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
CASES = [
    (
        "fallback-to-real",
        "vm.go",
        "res, err = reg.dryDummy(imp.fn.Sig.Returns)",
        "res, err = imp.fn.Fn(args)",
        "TestDryDefaultsSkipRealAndKeepResultSinkActive",
        None,
    ),
    (
        "double-call",
        "vm.go",
        "res, err = imp.fn.DryFn(args)",
        "res, err = imp.fn.DryFn(args); if err == nil { _, err = imp.fn.DryFn(args) }",
        "TestDryOverrideRunsOnceAndRealModeIgnoresIt",
        None,
    ),
    (
        "shared-mutable-dummy",
        "registry.go",
        'return r.dryCollections.object(declared, &IntList{})',
        'return r.dryCollections.object(declared, dryMutantIntList)',
        "TestDryDummiesUseDeclaredKindsAndFreshCollections",
        "package lmcode\n\nvar dryMutantIntList = &IntList{}\n",
    ),
]
with tempfile.TemporaryDirectory(prefix="go-dry-mutants-") as tmp:
    work = pathlib.Path(tmp) / "repo"
    shutil.copytree(ROOT, work, ignore=shutil.ignore_patterns(".git", "bin", "__pycache__"))

    def run(pattern):
        proc = subprocess.run(
            ["go", "test", ".", "-count=1", "-run", pattern],
            cwd=work,
            capture_output=True,
            text=True,
        )
        return proc.returncode, proc.stdout + proc.stderr

    status, output = run("^TestDry")
    assert status == 0, ("dry control baseline failed", output)
    print("PASS dry semantic control baseline", flush=True)
    originals = {filename: (work / filename).read_text() for _, filename, *_ in CASES}
    for name, filename, before, after, pattern, extra in CASES:
        for path, content in originals.items():
            (work / path).write_text(content)
        source = originals[filename]
        assert source.count(before) == 1, (name, "stale mutation anchor", source.count(before))
        (work / filename).write_text(source.replace(before, after, 1))
        if extra:
            (work / "dry-mutant.go").write_text(extra)
        status, output = run("^" + pattern + "$")
        assert status != 0 and "--- FAIL:" in output and "build failed" not in output and "panic:" not in output, (name, output)
        print("CAUGHT " + name, flush=True)
    print("PASS 3 dry semantic mutants", flush=True)
