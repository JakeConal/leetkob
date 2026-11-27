package sandbox

import (
	"context"
	"fmt"
)

// Analyze runs the local analyzer image under the same locked profile used for
// compilation. Source is copied into its private filesystem, never parsed on the
// host. The image contains the trusted /analyze-isolated entry point.
func (c *Controller) Analyze(ctx context.Context, source string) ([]byte, error) {
	if len(source) == 0 || len(source) > MaxSourceBytes {
		return nil, fmt.Errorf("%w: source size", ErrPolicy)
	}
	obs, _, err := c.run(ctx, runRequest{
		files:       map[string][]byte{"main.cpp": []byte(source)},
		command:     []string{"/analyze-isolated"},
		wall:        CompileWall,
		outputLimit: DefaultOutputBytes,
		profile:     compileProfile(),
	})
	if err != nil {
		return nil, err
	}
	if obs.ExitCode != 0 || obs.WallTimedOut || obs.OutputLimited {
		return nil, fmt.Errorf("sandbox: isolated analysis failed")
	}
	return obs.Stdout, nil
}
