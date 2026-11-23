package sandbox

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCompliantCpp17CompilesAndReturnsTokenData(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	source := attackSource(t, "compliant.cpp")

	result := judge(t, controller, JudgeRequest{
		Source:     source,
		CPUSeconds: 1,
		Tests: []TestCase{
			{ID: "sum", Stdin: []byte("3 4\n")},
			{ID: "negative", Stdin: []byte("-5 2\n")},
		},
	})

	if result.Verdict != VerdictRan {
		t.Fatalf("aggregate verdict = %s, want RAN (compile: %+v)", result.Verdict, result.Compile)
	}
	if !result.Compile.Succeeded {
		t.Fatalf("compile did not succeed: %+v", result.Compile)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("expected 2 test results, got %d", len(result.Tests))
	}
	wantTokens := [][]string{{"7"}, {"-3"}}
	for i, test := range result.Tests {
		if test.Verdict != VerdictRan {
			t.Fatalf("test %s verdict = %s, want RAN", test.ID, test.Verdict)
		}
		if strings.Join(test.Tokens, " ") != strings.Join(wantTokens[i], " ") {
			t.Fatalf("test %s tokens = %#v, want %#v", test.ID, test.Tokens, wantTokens[i])
		}
		if test.WallDeadline != 3*time.Second {
			t.Fatalf("test %s wall = %s, want 3s", test.ID, test.WallDeadline)
		}
		if test.Duration <= 0 {
			t.Fatalf("test %s has no recorded duration", test.ID)
		}
		assertContainerGone(t, test.Container)
	}
	assertContainerGone(t, result.Compile.Container)
}

func TestOutboundNetworkIsDenied(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "network_probe.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "network", Stdin: nil}},
	})
	test := singleTestResult(t, result)
	if test.Verdict != VerdictRan {
		t.Fatalf("verdict = %s, want RAN (the probe handles denial itself)", test.Verdict)
	}
	assertContains(t, test.Stdout, "DNS=denied", "CONNECT=denied")
	assertNotContains(t, test.Stdout, "DNS=resolved", "CONNECT=connected")
	assertContainerGone(t, test.Container)
}

func TestCPUInfiniteLoopIsTerminated(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	started := time.Now()
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "cpu_loop.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "cpu"}},
	})
	elapsed := time.Since(started)
	test := singleTestResult(t, result)

	if test.Verdict != VerdictTimeLimitExceeded {
		t.Fatalf("verdict = %s, want TLE", test.Verdict)
	}
	if result.Verdict != VerdictTimeLimitExceeded {
		t.Fatalf("aggregate verdict = %s, want TLE", result.Verdict)
	}
	if test.Duration < 2*time.Second {
		t.Fatalf("run ended after %s, before the 3s wall deadline", test.Duration)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("judging took %s, far beyond the wall deadline", elapsed)
	}
	assertContainerGone(t, test.Container)
}

func TestWallClockSleepIsTerminatedAndContainerRemoved(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "wall_sleep.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "wall"}},
	})
	test := singleTestResult(t, result)

	if test.Verdict != VerdictTimeLimitExceeded {
		t.Fatalf("verdict = %s, want TLE", test.Verdict)
	}
	assertNotContains(t, test.Stdout, "SLEPT=done")
	if test.Duration > 20*time.Second {
		t.Fatalf("wall sleep ran for %s; deadline was not enforced", test.Duration)
	}
	// The requirement is that the container is gone, not merely that the
	// Docker client exited.
	assertContainerGone(t, test.Container)
}

func TestMemoryAllocationIsBoundedWithReadBack(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "memory_overflow.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "memory"}},
	})
	test := singleTestResult(t, result)

	if test.Verdict != VerdictMemoryLimitExceeded {
		t.Fatalf("verdict = %s, want MLE (Docker did not report an OOM kill; exit=%d stderr=%s)",
			test.Verdict, test.ExitCode, test.Stderr)
	}
	// The fixture writes and reads every page back, so the allocation was
	// real. Unbuffered progress proves execution reached the ceiling.
	assertContains(t, test.Stdout, "ALLOC=", "READBACK=")
	assertNotContains(t, test.Stdout, "ALLOCATION_COMPLETED=unexpected")
	assertContainerGone(t, test.Container)
}

func TestProcessAndThreadCeilingIsBounded(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "process_limit.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "pids"}},
	})
	test := singleTestResult(t, result)

	if test.Verdict != VerdictRan {
		t.Fatalf("verdict = %s, want RAN (the fixture handles EAGAIN itself)", test.Verdict)
	}
	assertContains(t, test.Stdout, "THREAD_FAILED_AT=", "FORK_FAILED_AT=", "THREADS=", "CHILDREN=")
	assertNotContains(t, test.Stdout, "THREAD_FAILED_AT=none", "FORK_FAILED_AT=none")

	threads := intField(t, test.Stdout, "THREADS=")
	children := intField(t, test.Stdout, "CHILDREN=")
	if threads+children >= controller.Config().PidsLimit {
		t.Fatalf("created %d threads + %d children, reaching the %d PID ceiling",
			threads, children, controller.Config().PidsLimit)
	}
	if threads+children == 0 {
		t.Fatalf("no thread or child could be created; the fixture did not reach the ceiling")
	}
	assertContainerGone(t, test.Container)

	// A contained process attack must not disturb the next run.
	next := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "compliant.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "after-pids", Stdin: []byte("1 2\n")}},
	})
	nextTest := singleTestResult(t, next)
	if nextTest.Verdict != VerdictRan || strings.Join(nextTest.Tokens, " ") != "3" {
		t.Fatalf("run after PID attack = %+v, want RAN with token 3", nextTest)
	}
}

func TestOutputFloodIsBoundedAndMappedToRE(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "output_flood.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "flood"}},
	})
	test := singleTestResult(t, result)

	if test.Verdict != VerdictRuntimeError {
		t.Fatalf("verdict = %s, want RE for output overflow", test.Verdict)
	}
	if !test.OutputTruncated {
		t.Fatal("output was not reported as truncated")
	}
	if len(test.Stdout) > controller.Config().OutputBytes {
		t.Fatalf("collected %d stdout bytes, cap is %d", len(test.Stdout), controller.Config().OutputBytes)
	}
	if len(test.Stdout) != controller.Config().OutputBytes {
		t.Fatalf("collected %d stdout bytes, expected the full %d-byte cap", len(test.Stdout), controller.Config().OutputBytes)
	}
	if len(test.Tokens) > maxTokenCount {
		t.Fatalf("token list not bounded: %d", len(test.Tokens))
	}
	assertNotContains(t, test.Stdout, "FLOOD_DONE=1")
	assertContainerGone(t, test.Container)
}

func TestRootAndPrivilegeProbeIsUnprivileged(t *testing.T) {
	requireDocker(t)
	controller := testController(t)
	result := judge(t, controller, JudgeRequest{
		Source:     attackSource(t, "root_probe.cpp"),
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "root"}},
	})
	test := singleTestResult(t, result)

	if test.Verdict != VerdictRan {
		t.Fatalf("verdict = %s, want RAN (probe handles denial itself): %s", test.Verdict, test.Stderr)
	}
	assertContains(t, test.Stdout,
		"UID=65534",
		"EUID=65534",
		"SETUID0=denied",
		"CAPEFF=0000000000000000",
		"NONEWPRIVS=1",
		"SHADOW=denied",
		"ROOTFS_WRITE=denied",
		"USERNS=denied",
	)
	assertContainerGone(t, test.Container)
}

func TestNoHostMountsOrDockerSocket(t *testing.T) {
	requireDocker(t)
	controller := testController(t)

	canaryDir := t.TempDir()
	canaryPath := filepath.Join(canaryDir, "model.key")
	if err := writeCanary(canaryPath, "host-canary-not-mounted"); err != nil {
		t.Fatalf("write host canary: %v", err)
	}
	source := prelude(attackSource(t, "host_probe.cpp"),
		"#define LEETKOB_CANARY_PATH "+strconv.Quote(canaryPath))

	result := judge(t, controller, JudgeRequest{
		Source:     source,
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "host"}},
	})
	test := singleTestResult(t, result)

	if test.Verdict != VerdictRan {
		t.Fatalf("verdict = %s, want RAN: %s", test.Verdict, test.Stderr)
	}
	assertContains(t, test.Stdout,
		"DOCKER_SOCK_VAR_RUN=absent",
		"DOCKER_SOCK_RUN=absent",
		"CANARY_FILE=absent",
		"MOUNTINFO_BEGIN",
		"MOUNTINFO_END",
		" /work ",
	)
	assertNotContains(t, test.Stdout, "docker.sock", canaryDir)
	assertContainerGone(t, test.Container)
}

func TestNoSecretVisibleThroughEnvironmentFilesOrInclude(t *testing.T) {
	requireDocker(t)
	controller := testController(t)

	const canaryValue = "LEETKOB_SYNTHETIC_CANARY_9c1f0a"
	t.Setenv("LEETKOB_TEST_CANARY", canaryValue)

	canaryPath := filepath.Join(t.TempDir(), "model.key")
	if err := writeCanary(canaryPath, "#define LEETKOB_CANARY_VALUE \""+canaryValue+"\"\n"); err != nil {
		t.Fatalf("write canary file: %v", err)
	}
	source := prelude(attackSource(t, "secret_probe.cpp"),
		"#define LEETKOB_CANARY_PATH "+strconv.Quote(canaryPath),
		"#define LEETKOB_CANARY_ENV \"LEETKOB_TEST_CANARY\"",
	)

	result := judge(t, controller, JudgeRequest{
		Source:     source,
		CPUSeconds: 1,
		Tests:      []TestCase{{ID: "secrets"}},
	})
	test := singleTestResult(t, result)
	if test.Verdict != VerdictRan {
		t.Fatalf("verdict = %s, want RAN: %s", test.Verdict, test.Stderr)
	}
	assertContains(t, test.Stdout,
		"ENV_CANARY=absent",
		"INCLUDE_CANARY=absent",
		"FILE_CANARY=absent",
		"CANDIDATE /etc/leetkob/model.key=absent",
		"CANDIDATE /run/secrets/model_key=absent",
		"ENVIRON_BEGIN",
		"ENVIRON_END",
	)
	combined := result.Compile.Stdout + result.Compile.Stderr + test.Stdout + test.Stderr
	assertNotContains(t, combined, canaryValue)
	assertContainerGone(t, test.Container)
}

func intField(t *testing.T, output, prefix string) int {
	t.Helper()
	index := strings.Index(output, prefix)
	if index < 0 {
		t.Fatalf("output missing %q\n%s", prefix, output)
	}
	rest := strings.TrimSpace(output[index+len(prefix):])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		t.Fatalf("no value after %q\n%s", prefix, output)
	}
	value, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("parse %q from %q: %v", prefix, fields[0], err)
	}
	return value
}

func writeCanary(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
