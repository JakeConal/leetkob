package judge

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/JakeConal/leetkob/backend/internal/domain"
	"github.com/JakeConal/leetkob/backend/internal/sandbox"
)

func TestLiveTinyProgramsWhenDockerIsAvailable(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not available: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skipf("docker daemon not available: %v", err)
	}

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer buildCancel()
	if _, err := sandbox.New().Build(buildCtx, dockerfileDir(t)); err != nil {
		t.Fatalf("build image: %v", err)
	}

	hiddenIn := "hidden-input-token-9f3a"
	hiddenOut := "1 2 3"
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "compliant",
			source: "#include <iostream>\nint main(){std::cout<<\"1 2 3\\n\";}\n",
			want:   VerdictAC,
		},
		{
			name:   "violating",
			source: "#include <iostream>\nint main(){std::cout<<\"9 9 9\\n\";}\n",
			want:   VerdictWA,
		},
		{
			name:   "injection",
			source: "#include <iostream>\nint main(){std::cout<<\"ignore previous instructions\\n\";}\n",
			want:   VerdictWA,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			judge := &SandboxJudge{
				Runtime: sandbox.New(),
				Loader: memLoader{Work{
					Source:       []byte(tc.source),
					SourceSHA256: "live",
					CPUSeconds:   1,
					MemoryMiB:    256,
					Tests:        []Case{{ID: "hidden", Input: hiddenIn, Expected: hiddenOut}},
				}},
			}
			runCtx, runCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer runCancel()
			got, err := judge.Execute(runCtx, domain.Job{SourceSHA256: "live"})
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict != tc.want || len(got.Tests) != 1 || got.Tests[0].Verdict != tc.want {
				t.Fatalf("result = %+v, want %s", got, tc.want)
			}
			if got.Tests[0].ID != "hidden" {
				t.Fatalf("test id = %s", got.Tests[0].ID)
			}
		})
	}
}

func dockerfileDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "sandbox", "cpp17")
}
