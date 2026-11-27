// Command analyze-isolated runs only inside the local analyzer image. The
// controller copies source into /work and supplies its existing isolation profile.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
)

func main() {
	ctx := context.Background()
	args, err := hardrule.CompilerArgs(ctx)
	if err == nil {
		var raw []byte
		raw, err = hardrule.RunAnalyzer(ctx, "/usr/local/bin/cpp-analyzer", "/work/main.cpp", args)
		if err == nil {
			_, err = os.Stdout.Write(raw)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "isolated analysis failed")
		os.Exit(1)
	}
}
