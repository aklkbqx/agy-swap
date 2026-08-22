package main

import (
	"context"
	"os"

	"github.com/aklkbqx/agy-swap/internal/app"
)

var version = "2.1.2-dev"
var buildID = "dev"

func main() {
	application, err := app.New(version, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		_, _ = os.Stderr.WriteString("agy-swap: " + err.Error() + "\n")
		os.Exit(1)
	}
	application.BuildID = buildID
	os.Exit(application.Run(context.Background(), os.Args[1:]))
}
