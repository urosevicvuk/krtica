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

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cli.New().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "krtica:", err)
		return 1
	}
	return 0
}
