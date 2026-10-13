//go:build !windows

package app

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// prepareLoginCommand starts agy in its own process group so it can be stopped
// as a tree. agy prompts in the terminal, and a background group is stopped by
// SIGTTIN/SIGTTOU there, so while it runs that group owns the terminal;
// reclaimLoginTerminal hands it back.
func prepareLoginCommand(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	if fd, ok := foregroundTerminal(command.Stdin); ok {
		command.SysProcAttr.Foreground = true
		command.SysProcAttr.Ctty = fd
	}
	if command.Cancel != nil {
		command.Cancel = func() error {
			if command.Process == nil {
				return os.ErrProcessDone
			}
			return unix.Kill(-command.Process.Pid, unix.SIGINT)
		}
	}
}

func stopLoginProcessGroup(command *exec.Cmd) bool {
	if command == nil || command.Process == nil {
		return false
	}
	return unix.Kill(-command.Process.Pid, unix.SIGINT) == nil
}

func killLoginCommandTree(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = unix.Kill(-command.Process.Pid, unix.SIGKILL)
	_ = command.Process.Kill()
}

// foregroundTerminal returns the descriptor of in when it is agy-swap's
// controlling terminal and agy-swap's process group owns it. Only then can the
// terminal be handed to agy and taken back.
func foregroundTerminal(in io.Reader) (int, bool) {
	file, ok := in.(*os.File)
	if !ok {
		return 0, false
	}
	fd := int(file.Fd())
	owner, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	return fd, err == nil && owner == unix.Getpgrp()
}

// reclaimLoginTerminal makes agy-swap's process group own the terminal again
// after agy exits, as a shell does when a job ends. agy-swap is a background
// process until the call succeeds, so SIGTTOU is ignored for its duration.
func reclaimLoginTerminal(command *exec.Cmd) {
	if command.SysProcAttr == nil || !command.SysProcAttr.Foreground {
		return
	}
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	_ = unix.IoctlSetPointerInt(command.SysProcAttr.Ctty, unix.TIOCSPGRP, unix.Getpgrp())
}
