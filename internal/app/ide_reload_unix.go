//go:build !windows

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func signalProcessTerm(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func detectAntigravityIDERunning() bool {
	pgrepBin, err := exec.LookPath("pgrep")
	if err != nil || pgrepBin == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{}
	if uid := os.Getuid(); uid >= 0 {
		args = append(args, "-u", strconv.Itoa(uid))
	}
	if runtime.GOOS == "darwin" {
		args = append(args, "-f", `Antigravity IDE\.app/Contents/MacOS/Electron`)
	} else {
		args = append(args, "-f", `antigravity-ide`)
	}
	return exec.CommandContext(ctx, pgrepBin, args...).Run() == nil
}

func captureWindowsIDEExecutable() string { return "" }

func gracefulQuitAntigravityIDE(ctx context.Context) error {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(qCtx, "osascript", "-e", `tell application id "com.google.antigravity-ide" to quit`).Run()
	}
	pkillBin, err := exec.LookPath("pkill")
	if err != nil {
		return err
	}
	args := []string{"-TERM"}
	if uid := os.Getuid(); uid >= 0 {
		args = append(args, "-u", strconv.Itoa(uid))
	}
	args = append(args, "-f", `antigravity-ide`)
	return exec.CommandContext(qCtx, pkillBin, args...).Run()
}

func relaunchAntigravityIDE(ctx context.Context, _ string) error {
	rCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if runtime.GOOS == "darwin" {
		// -g launches in background without stealing focus; -b targets bundle ID.
		return exec.CommandContext(rCtx, "open", "-g", "-b", "com.google.antigravity-ide").Run()
	}
	bin, err := exec.LookPath("antigravity-ide")
	if err != nil {
		return err
	}
	cmd := exec.Command(bin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
