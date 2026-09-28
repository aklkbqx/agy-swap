package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aklkbqx/agy-swap/internal/app"
)

var version = "2.6.1"

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	application, err := app.NewDemo(version, os.Stdin, os.Stdout, os.Stderr, app.DemoOptions{Home: home})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(application.Run(context.Background(), nil))
}
