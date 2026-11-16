package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/xrzks/decal/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version string

func main() {
	err := cli.New(buildVersion()).Run(context.Background(), os.Args)
	switch {
	case errors.Is(err, cli.ErrCancelled):
		os.Exit(cli.ExitCancelled)
	case err != nil:
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// buildVersion prefers the version stamped by -ldflags, then the module or VCS
// version the Go toolchain records in the binary.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
