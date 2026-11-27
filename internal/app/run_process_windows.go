//go:build windows

package app

import (
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type managedChildHandle struct {
	job windows.Handle
}

func configureManagedChild(cmd *exec.Cmd, _ bool) *managedChildHandle {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	return &managedChildHandle{}
}

func (h *managedChildHandle) attachStarted(cmd *exec.Cmd) {
	if h == nil || cmd == nil || cmd.Process == nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, _ = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	h.job = job
}

func (h *managedChildHandle) close() {
	if h != nil && h.job != 0 {
		_ = windows.CloseHandle(h.job)
		h.job = 0
	}
}

func (h *managedChildHandle) signalGraceful(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// CTRL_BREAK_EVENT targets the child process group created with CREATE_NEW_PROCESS_GROUP.
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)); err != nil {
		h.forceKill(cmd)
	}
}

func (h *managedChildHandle) forceKill(cmd *exec.Cmd) {
	if h != nil && h.job != 0 {
		_ = windows.TerminateJobObject(h.job, 1)
		return
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

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
