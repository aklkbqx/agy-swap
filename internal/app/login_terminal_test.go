//go:build darwin || linux

package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const loginTerminalHelperEnv = "AGY_SWAP_LOGIN_TERMINAL_HELPER"

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
