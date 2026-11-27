//go:build windows

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

func signalProcessTerm(pid int) error {
	if pid <= 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func detectAntigravityIDERunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tasklist", "/FI", "IMAGENAME eq Antigravity IDE.exe", "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "antigravity ide.exe")
}

func captureWindowsIDEExecutable() string {
	candidates := []string{}
	if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
		candidates = append(candidates, filepath.Join(localApp, "Programs", "Antigravity IDE", "Antigravity IDE.exe"))
	}
	if progFiles := os.Getenv("ProgramFiles"); progFiles != "" {
		candidates = append(candidates, filepath.Join(progFiles, "Antigravity IDE", "Antigravity IDE.exe"))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

func gracefulQuitAntigravityIDE(ctx context.Context) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// taskkill without /F sends WM_CLOSE gracefully so VS Code hotExit runs.
	return exec.CommandContext(qCtx, "taskkill", "/IM", "Antigravity IDE.exe").Run()
}

func relaunchAntigravityIDE(_ context.Context, exePath string) error {
	if exePath == "" {
		exePath = captureWindowsIDEExecutable()
	}
	if exePath == "" {
		return errors.New("could not locate Antigravity IDE.exe to relaunch")
	}
	cmd := exec.Command(exePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	return cmd.Start()
}

func spawnDetachedIDERestart(email string, isGCPTos bool) error {
	self := currentSwapExecutable()
	if self == "" {
		return errors.New("cannot resolve current agy-swap binary")
	}
	gcpFlag := "--gcp-tos=false"
	if isGCPTos {
		gcpFlag = "--gcp-tos=true"
	}
	cmd := exec.Command(self, "__internal-restart-ide", "--email", email, gcpFlag)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
