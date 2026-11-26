//go:build !windows

package app

import (
	"os"
	"os/signal"
	"syscall"
)

func watchTerminalResize() (<-chan struct{}, func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	out := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigCh:
				select {
				case out <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()
	stop := func() {
		signal.Stop(sigCh)
		close(done)
	}
	return out, stop
}
