//go:build darwin || linux

package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const (
	loginTerminalHelperEnv    = "AGY_SWAP_LOGIN_TERMINAL_HELPER"
	tuiDirtyLoginHelperEnv    = "AGY_SWAP_TUI_DIRTY_LOGIN_HELPER"
	tuiDirtyLoginTriggerEnv   = "AGY_SWAP_TUI_DIRTY_LOGIN_TRIGGER"
	tuiDirtyLoginRawChildFlag = "AGY_SWAP_DIRTY_RAW_CHILD"
)

// The login child must own the terminal while it runs (it prompts and changes
// terminal modes), and agy-swap must own it again as soon as the child exits:
// the TUI switches back to raw mode right after a login.
func TestLoginCommandGetsAndReturnsTheTerminal(t *testing.T) {
	master, slave := openPTY(t)
	defer func() { _ = master.Close() }()
	helper := exec.Command(os.Args[0], "-test.run=^TestLoginTerminalHelperProcess$")
	helper.Env = append(os.Environ(), loginTerminalHelperEnv+"=1")
	helper.Stdin, helper.Stdout, helper.Stderr = slave, slave, slave
	// A new session with the pseudo-terminal as its controlling terminal, as
	// in a real terminal window.
	helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()
	var output bytes.Buffer
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			mu.Lock()
			output.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	read := func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.ReplaceAll(output.String(), "\r", "")
	}
	// Answer the "Press Enter when ready" prompt like a user would.
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(read(), "Press Enter") && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	_, _ = master.Write([]byte("\r"))
	done := make(chan error, 1)
	go func() { done <- helper.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		<-done
		t.Fatal("helper did not finish")
	}
	time.Sleep(50 * time.Millisecond)
	text := read()
	if !strings.Contains(text, "child-done") || !strings.Contains(text, "terminal-returned") {
		t.Fatalf("login terminal handoff failed:\n%s", text)
	}
}

// TestLoginTerminalHelperProcess runs inside the pseudo-terminal session
// started by TestLoginCommandGetsAndReturnsTheTerminal. It drives the real add
// flow with a fake credential store, then switches to raw mode as the TUI does
// after a login.
func TestLoginTerminalHelperProcess(t *testing.T) {
	if os.Getenv(loginTerminalHelperEnv) != "1" {
		t.Skip("runs only as the helper of TestLoginCommandGetsAndReturnsTheTerminal")
	}
	f := newCLIFixture(t)
	// Stand-in for agy: it changes terminal modes like an interactive prompt
	// and exits without saving a credential.
	f.a.loginCommand = func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "stty -echo; stty echo; echo child-done")
	}
	f.a.In, f.a.Out, f.a.Err = os.Stdin, os.Stdout, os.Stdout
	f.a.lineReader = nil
	done := make(chan int, 1)
	go func() { done <- f.a.addLoginFlow(context.Background()) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		fmt.Println("child-stopped: the terminal stopped the login command")
		os.Exit(1)
	}
	owner, err := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	if err != nil || owner != unix.Getpgrp() {
		fmt.Printf("terminal-lost: owner %d, agy-swap group %d, %v\n", owner, unix.Getpgrp(), err)
		os.Exit(1)
	}
	state, err := term.MakeRaw(0)
	if err != nil {
		fmt.Println("raw-failed:", err)
		os.Exit(1)
	}
	_ = term.Restore(0, state)
	fmt.Println("terminal-returned")
	os.Exit(0)
}

func TestLoginCommandWithoutTerminalKeepsItsOwnGroup(t *testing.T) {
	command := exec.Command("true")
	command.Stdin = strings.NewReader("")
	prepareLoginCommand(command)
	if !command.SysProcAttr.Setpgid || command.SysProcAttr.Foreground {
		t.Fatalf("non-terminal stdin: %+v", command.SysProcAttr)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	command = exec.Command("true")
	command.Stdin = reader
	prepareLoginCommand(command)
	if command.SysProcAttr.Foreground {
		t.Fatal("a pipe is not a terminal to hand over")
	}
}

func TestTUISuspendPreservesCookedTerminalAfterRawLoginChild(t *testing.T) {
	master, slave := openPTY(t)
	defer func() { _ = master.Close() }()
	triggerFile := t.TempDir() + "/seed-second-session"
	helper := exec.Command(os.Args[0], "-test.run=^TestTUISuspendDirtyLoginHelperProcess$")
	helper.Env = append(os.Environ(),
		tuiDirtyLoginHelperEnv+"=1",
		tuiDirtyLoginTriggerEnv+"="+triggerFile,
	)
	helper.Stdin, helper.Stdout, helper.Stderr = slave, slave, slave
	helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()

	var rawOutput bytes.Buffer
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				mu.Lock()
				rawOutput.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	snapshot := func() string {
		mu.Lock()
		defer mu.Unlock()
		return rawOutput.String()
	}
	waitFor := func(substr string, timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if strings.Contains(snapshot(), substr) {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return false
	}

	// Wait for initial TUI alternate screen entry, then press 'a' for first login.
	if !waitFor("\x1b[?1049h", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("TUI did not enter alternate screen:\n%q", snapshot())
	}
	time.Sleep(80 * time.Millisecond)
	_, _ = master.Write([]byte("a"))

	if !waitFor("Press Enter when ready", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("first add flow did not prompt:\n%q", snapshot())
	}
	// Trigger the child that leaves the terminal in raw mode + alternate screen without restoring.
	_, _ = master.Write([]byte("\r"))

	if !waitFor("dirty-child-started", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("dirty login child did not run:\n%q", snapshot())
	}
	if !waitFor("exited before saving a login credential", 5*time.Second) || !waitFor("Action failed", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("first add flow did not finish and return to the TUI before seeding second session:\n%q", snapshot())
	}
	// Signal helper to seed an unsaved active session token for the second 'a' press.
	if err := os.WriteFile(triggerFile, []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	seededFile := triggerFile + ".done"
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(seededFile); err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if _, err := os.Stat(seededFile); err != nil {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("helper did not finish seeding second session token: %v", err)
	}
	_, _ = master.Write([]byte("a"))

	if !waitFor("Save this account? [y/N]", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("second add flow did not show Save prompt:\n%q", snapshot())
	}

	beforeAnswer := snapshot()
	idx := strings.LastIndex(beforeAnswer, "This account is not yet saved in agy-swap.")
	if idx < 0 || !strings.Contains(beforeAnswer[idx:], "\r\n") {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("second prompt lost cooked ONLCR carriage-return translation (staircase output):\n%q", beforeAnswer[max(0, idx-40):])
	}

	// Press 'y' + Enter ('\r', as a real terminal keyboard sends).
	_, _ = master.Write([]byte("y\r"))
	if !waitFor("Successfully saved account: Google User <second@example.com>", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("pressing y + Enter hung or failed after dirty login child:\n%q", snapshot())
	}

	time.Sleep(120 * time.Millisecond)
	_, _ = master.Write([]byte("q"))

	done := make(chan error, 1)
	go func() { done <- helper.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exited with error %v:\n%q", err, snapshot())
		}
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		<-done
		t.Fatalf("TUI helper did not exit cleanly on q:\n%q", snapshot())
	}
}

func TestTUISuspendDirtyLoginHelperProcess(t *testing.T) {
	if os.Getenv(tuiDirtyLoginRawChildFlag) == "1" {
		// Simulate interactive agy on Windows when terminated by taskkill /F:
		// enters alternate screen and raw terminal mode, then exits abruptly without restoring.
		_, _ = term.MakeRaw(0)
		fmt.Print("\x1b[?1049hdirty-child-started")
		os.Exit(1)
	}
	if os.Getenv(tuiDirtyLoginHelperEnv) != "1" {
		t.Skip("runs only as helper of TestTUISuspendPreservesCookedTerminalAfterRawLoginChild")
	}
	f := newCLIFixture(t)
	f.userInfo = "second@example.com"
	f.a.quota = NewQuotaService(f.a.http, f.a.store)
	f.a.quota.SetVault(f.vault)
	token := tokenBlob(t, "second@example.com", true, "refresh-second", time.Now().Add(time.Hour))
	triggerFile := os.Getenv(tuiDirtyLoginTriggerEnv)
	f.a.loginCommand = func(ctx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTUISuspendDirtyLoginHelperProcess$")
		cmd.Env = append(os.Environ(), tuiDirtyLoginRawChildFlag+"=1")
		return cmd
	}
	go func() {
		for {
			if _, err := os.Stat(triggerFile); err == nil {
				_ = f.a.credentials.writeOAuthFiles(token, "second@example.com")
				_ = os.WriteFile(triggerFile+".done", []byte("1"), 0o600)
				return
			}
			time.Sleep(15 * time.Millisecond)
		}
	}()
	f.a.In, f.a.Out, f.a.Err = os.Stdin, os.Stdout, os.Stdout
	f.a.stdinTTY, f.a.stdoutTTY = true, true
	f.a.lineReader = nil
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := f.a.Run(ctx, nil)
	cancel()
	os.Exit(code)
}

func TestTUICtrlCAtAddLoginPromptReturnsToDashboard(t *testing.T) {
	master, slave := openPTY(t)
	defer func() { _ = master.Close() }()
	triggerFile := t.TempDir() + "/unused-trigger"
	helper := exec.Command(os.Args[0], "-test.run=^TestTUISuspendDirtyLoginHelperProcess$")
	helper.Env = append(os.Environ(),
		tuiDirtyLoginHelperEnv+"=1",
		tuiDirtyLoginTriggerEnv+"="+triggerFile,
	)
	helper.Stdin, helper.Stdout, helper.Stderr = slave, slave, slave
	helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()

	var rawOutput bytes.Buffer
	var mu sync.Mutex
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				mu.Lock()
				rawOutput.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	snapshot := func() string {
		mu.Lock()
		defer mu.Unlock()
		return rawOutput.String()
	}
	waitFor := func(substr string, timeout time.Duration) bool {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if strings.Contains(snapshot(), substr) {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return false
	}

	if !waitFor("\x1b[?1049h", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("TUI did not enter alternate screen:\n%q", snapshot())
	}
	time.Sleep(80 * time.Millisecond)
	_, _ = master.Write([]byte("a"))

	if !waitFor("Press Enter when ready", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("add flow did not prompt:\n%q", snapshot())
	}
	// Send Ctrl+C on the controlling terminal while at the sign-in prompt.
	_, _ = master.Write([]byte{0x03})

	if !waitFor("Login cancelled.", 5*time.Second) {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("Ctrl+C at sign-in prompt did not print 'Login cancelled.':\n%q", snapshot())
	}
	// After Login cancelled., the TUI must re-enter the alternate screen instead of exiting.
	deadline := time.Now().Add(5 * time.Second)
	returnedToTUI := false
	for time.Now().Before(deadline) {
		out := snapshot()
		idx := strings.LastIndex(out, "Login cancelled.")
		if idx >= 0 && strings.Contains(out[idx:], "\x1b[?1049h") && !strings.Contains(out[idx+strings.Index(out[idx:], "\x1b[?1049h"):], "\x1b[?1049l") {
			returnedToTUI = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !returnedToTUI {
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		t.Fatalf("TUI exited instead of returning to the account list after Ctrl+C at prompt:\n%q", snapshot())
	}

	// Ensure the TUI still responds to subsequent keys without any leaked readLine goroutine stealing input.
	time.Sleep(80 * time.Millisecond)
	_, _ = master.Write([]byte("q"))

	done := make(chan error, 1)
	go func() { done <- helper.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exited with error %v:\n%q", err, snapshot())
		}
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL)
		<-done
		t.Fatalf("TUI helper did not exit cleanly on q after Ctrl+C cancellation:\n%q", snapshot())
	}
}
