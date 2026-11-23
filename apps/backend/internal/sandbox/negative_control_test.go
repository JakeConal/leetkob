package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNegativeControlDetectsMissingNonRootEnforcement proves that the
// non-root assertion is observable, not decorative. The root probe runs
// once through the real controller, which must report UID 65534, and once
// through an isolated test-only helper that deliberately overrides the
// user to 0:0. The helper must report UID 0, showing the test can detect
// absent enforcement. The helper is not part of Controller or Config and
// never weakens the real configuration.
func TestNegativeControlDetectsMissingNonRootEnforcement(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	source := attackSource(t, "root_probe.cpp")

	contained := judge(t, controller, JudgeRequest{
		Source:     source,
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "root-contained"}},
	})
	containedTest := singleTestResult(t, contained)
	if containedTest.Verdict != VerdictRan {
		t.Fatalf("contained probe verdict = %s: %s", containedTest.Verdict, containedTest.Stderr)
	}
	assertContains(t, containedTest.Stdout, "UID=65534")

	unconfined := runUnconfinedProbe(t, source)
	assertContains(t, unconfined, "UID=0")
	assertNotContains(t, unconfined, "UID=65534")
}

// runUnconfinedProbe is a deliberate negative control. It runs the same
// fixed image and fixture with --user 0:0 so the non-root control is
// absent, and with no capability/memory/PID/network hardening. It exists
// only in the test binary and only returns the probe's observations.
func runUnconfinedProbe(t *testing.T, source []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod work dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.cpp"), source, 0o644); err != nil {
		t.Fatalf("write probe source: %v", err)
	}
	mount := "type=bind,src=" + dir + ",dst=/work"

	compile := exec.Command(
		"docker", "run", "--rm",
		"--user", "0:0",
		"--mount", mount,
		"--workdir", "/work",
		sandboxImage,
		"g++", "-O2", "-std=c++17", "-o", "/work/program", "/work/main.cpp",
	)
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("negative-control compile: %v\n%s", err, out)
	}

	run := exec.Command(
		"docker", "run", "--rm",
		"--user", "0:0",
		"--mount", mount+",readonly",
		"--workdir", "/work",
		sandboxImage,
		harnessPath,
	)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("negative-control run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "UID=") {
		t.Fatalf("negative-control probe did not report an identity:\n%s", out)
	}
	return string(out)
}
