package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/aklkbqx/agy-swap/v2/internal/app"
)

var version = "2.23.3"
var buildID = "dev"

func main() {
	application, err := app.New(version, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		_, _ = os.Stderr.WriteString("agy-swap: " + err.Error() + "\n")
		os.Exit(1)
	}
	application.BuildID = buildID

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	code := application.Run(ctx, os.Args[1:])
	cancel()
	os.Exit(code)
}
