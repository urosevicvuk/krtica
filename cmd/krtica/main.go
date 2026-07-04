// Command krtica is the single binary for every krtica role (docs/DESIGN.md
// §14): `krtica server`, `krtica agent`, and control-API subcommands.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urosevicvuk/krtica/internal/cli"
)

func main() {
	os.Exit(run())
}

// run exists so deferred cleanup executes before the process exits;
// os.Exit directly in main would skip defers.
func run() int {
	// SIGINT/SIGTERM cancel the context, giving server and agent their
	// graceful-shutdown signal; a second signal kills the process hard.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cli.New().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "krtica:", err)
		return 1
	}
	return 0
}
