//go:build !windows

package app

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func prepareLoginCommand(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
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
