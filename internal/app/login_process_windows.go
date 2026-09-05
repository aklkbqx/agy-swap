//go:build windows

package app

import (
	"os"
	"os/exec"
	"strconv"
)

func prepareLoginCommand(command *exec.Cmd) {
	if command.Cancel != nil {
		command.Cancel = func() error {
			if command.Process == nil {
				return os.ErrProcessDone
			}
			return exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F").Run()
		}
	}
}

func stopLoginProcessGroup(*exec.Cmd) bool { return false }

func killLoginCommandTree(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F").Run()
	_ = command.Process.Kill()
}
