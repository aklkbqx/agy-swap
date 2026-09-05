package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedCredentialBackend struct {
	mu    sync.Mutex
	token string
}

func (f *lockedCredentialBackend) Get(context.Context) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.token
}

func (f *lockedCredentialBackend) Set(_ context.Context, value string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = value
	return true
}

func (f *lockedCredentialBackend) Delete(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = ""
	return true
}

func TestLoginCommandHelper(t *testing.T) {
	switch os.Getenv("AGY_SWAP_LOGIN_HELPER") {
	case "exit":
		fmt.Fprintln(os.Stdout, "interactive child stdout")
		fmt.Fprintln(os.Stderr, "interactive child stderr")
		os.Exit(23)
	case "wait-for-interrupt":
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		defer signal.Stop(interrupt)
		if err := os.WriteFile(os.Getenv("AGY_SWAP_LOGIN_READY_FILE"), []byte("ready"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(24)
		}
		<-interrupt
		fmt.Fprintln(os.Stdout, "interactive child cleaned up")
		os.Exit(0)
	default:
		return
	}
}

func TestAddLoginFlowAttachesIOAndStopsWhenAgyExits(t *testing.T) {
	paths := testPaths(t)
	input := bytes.NewBufferString("\n")
	var output, errorOutput bytes.Buffer
	previousToken := tokenBlob(t, "previous@example.com", true, "previous-refresh", time.Now().Add(time.Hour))
	backend := &fakeCredentialBackend{token: previousToken}
	credentials := NewCredentials(paths)
	credentials.backend = backend
	store := NewStore(paths)
	accounts := NewAccounts()
	accounts.Set("previous@example.com", newAccount("previous@example.com", "Previous User", previousToken))
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	var command *exec.Cmd
	a := &Application{
		In:                input,
		Out:               &output,
		Err:               &errorOutput,
		lineReader:        bufio.NewReader(input),
		paths:             paths,
		store:             store,
		credentials:       credentials,
		p:                 makePalette(false),
		loginTimeout:      10 * time.Second,
		loginPollInterval: 10 * time.Millisecond,
		loginCommand: func(ctx context.Context) *exec.Cmd {
			command = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLoginCommandHelper$")
			command.Env = append(os.Environ(), "AGY_SWAP_LOGIN_HELPER=exit")
			return command
		},
	}

	started := time.Now()
	if code := a.addLoginFlow(context.Background()); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if elapsed := time.Since(started); elapsed >= 3*time.Second {
		t.Fatalf("waited for timeout after child exit: %s", elapsed)
	}
	if command == nil {
		t.Fatal("login command was not created")
	}
	if command.Stdin != a.In || command.Stdout != a.Out || command.Stderr != a.Err {
		t.Fatal("login command was not attached to application I/O")
	}
	if !strings.Contains(output.String(), "interactive child stdout") {
		t.Fatalf("child stdout was hidden: %q", output.String())
	}
	if !strings.Contains(errorOutput.String(), "interactive child stderr") {
		t.Fatalf("child stderr was hidden: %q", errorOutput.String())
	}
	if !strings.Contains(errorOutput.String(), "exited before saving a login credential") {
		t.Fatalf("missing early-exit error: %q", errorOutput.String())
	}
	if strings.Contains(output.String(), "Timed out waiting for login") {
		t.Fatalf("reported timeout after child exit: %q", output.String())
	}
	if backend.token != previousToken {
		t.Fatalf("previous credential was not restored: %q", backend.token)
	}
}

func TestStopLoginCommandLetsWrapperAndChildCleanUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell wrapper regression")
	}
	tempDir := t.TempDir()
	readyPath := filepath.Join(tempDir, "child-ready")
	wrapperPath := filepath.Join(tempDir, "agy")
	wrapper := "#!/bin/sh\n\"$AGY_SWAP_TEST_BINARY\" -test.run=^TestLoginCommandHelper$\nstatus=$?\nprintf 'wrapper cleaned up\\n'\nexit \"$status\"\n"
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	command := exec.Command(wrapperPath)
	command.Env = append(os.Environ(),
		"AGY_SWAP_TEST_BINARY="+os.Args[0],
		"AGY_SWAP_LOGIN_HELPER=wait-for-interrupt",
		"AGY_SWAP_LOGIN_READY_FILE="+readyPath,
	)
	command.Stdout = &output
	command.Stderr = &output
	prepareLoginCommand(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			killLoginCommandTree(command)
			t.Fatal("wrapped child did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stopLoginCommand(command, done)
	if command.ProcessState == nil || !command.ProcessState.Success() {
		t.Fatalf("wrapper did not exit cleanly: %v; output=%q", command.ProcessState, output.String())
	}
	if got := output.String(); !strings.Contains(got, "interactive child cleaned up") || !strings.Contains(got, "wrapper cleaned up") {
		t.Fatalf("process tree did not clean up in order: %q", got)
	}
}

func TestAddLoginFlowSavesCredentialAndCleansUpWrapper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell wrapper regression")
	}
	paths := testPaths(t)
	tempDir := t.TempDir()
	readyPath := filepath.Join(tempDir, "child-ready")
	wrapperPath := filepath.Join(tempDir, "agy")
	wrapper := "#!/bin/sh\n\"$AGY_SWAP_TEST_BINARY\" -test.run=^TestLoginCommandHelper$\nstatus=$?\nprintf 'wrapper cleaned up\\n'\nexit \"$status\"\n"
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}

	var output, errorOutput bytes.Buffer
	backend := &lockedCredentialBackend{}
	credentials := NewCredentials(paths)
	credentials.backend = backend
	httpService := NewHTTPService(&errorOutput)
	httpService.userInfoURL = "://invalid"
	vault := fakeAccountVault{}
	a := &Application{
		In:                bytes.NewBufferString("\n"),
		Out:               &output,
		Err:               &errorOutput,
		lineReader:        bufio.NewReader(bytes.NewBufferString("\n")),
		paths:             paths,
		store:             NewStore(paths),
		credentials:       credentials,
		vault:             vault,
		http:              httpService,
		p:                 makePalette(false),
		loginTimeout:      5 * time.Second,
		loginPollInterval: 10 * time.Millisecond,
		loginCommand: func(ctx context.Context) *exec.Cmd {
			command := exec.CommandContext(ctx, wrapperPath)
			command.Env = append(os.Environ(),
				"AGY_SWAP_TEST_BINARY="+os.Args[0],
				"AGY_SWAP_LOGIN_HELPER=wait-for-interrupt",
				"AGY_SWAP_LOGIN_READY_FILE="+readyPath,
			)
			return command
		},
	}
	a.lineReader = bufio.NewReader(a.In)
	token := tokenBlob(t, "user@example.com", true, "refresh", time.Now().Add(time.Hour))
	credentialSet := make(chan struct{})
	go func() {
		defer close(credentialSet)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(readyPath); err == nil {
				backend.Set(context.Background(), token)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if code := a.addLoginFlow(context.Background()); code != 0 {
		t.Fatalf("exit code=%d output=%q error=%q", code, output.String(), errorOutput.String())
	}
	<-credentialSet
	if got := output.String(); !strings.Contains(got, "interactive child cleaned up") || !strings.Contains(got, "wrapper cleaned up") {
		t.Fatalf("login process tree did not clean up: %q", got)
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	account, ok := accounts.Get("user@example.com")
	if !ok {
		t.Fatalf("saved account missing: %#v", accounts)
	}
	ref := getString(account, "secret_ref")
	if ref == "" || getString(account, "token_data") != "" || vault[ref] != token {
		t.Fatalf("credential was not saved to the vault: account=%#v", account)
	}
}
