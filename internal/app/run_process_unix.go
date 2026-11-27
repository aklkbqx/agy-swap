//go:build !windows

package app

import (
	"os/exec"
	"syscall"
	"time"
)

type managedChildHandle struct {
	pgidMode bool
}

// configureManagedChild prepares a child command for graceful draining. When
// running non-interactively (no foreground TTY), it isolates the child in its
// own process group so drainManagedChild can signal the full tree cleanly
// without risking SIGTTIN on interactive terminal sessions.
func configureManagedChild(cmd *exec.Cmd, interactiveTTY bool) *managedChildHandle {
	h := &managedChildHandle{pgidMode: !interactiveTTY}
	if h.pgidMode {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return h
}

func (h *managedChildHandle) attachStarted(_ *exec.Cmd) {}

func (h *managedChildHandle) close() {}

func (h *managedChildHandle) signalGraceful(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if h != nil && h.pgidMode {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
}

func (h *managedChildHandle) forceKill(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if h != nil && h.pgidMode {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}

// drainManagedChild gracefully terminates cmd and waits up to grace before
// escalating to SIGKILL, guaranteeing zero process overlap before swapping.
func drainManagedChild(cmd *exec.Cmd, handle *managedChildHandle, waitCh <-chan error, grace time.Duration) error {
	if grace <= 0 {
		grace = 5 * time.Second
	}
	handle.signalGraceful(cmd)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-waitCh:
		return err
	case <-timer.C:
		handle.forceKill(cmd)
		return <-waitCh
	}
}
