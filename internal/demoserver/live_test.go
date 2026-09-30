package demoserver

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLiveNativeTUISwitchAndCleanup(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agy-swap-demo")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/agy-swap-demo")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build demo: %v: %s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	root := t.TempDir()
	handler := New(Config{Origin: origin, Binary: binary, TempRoot: root})
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Shutdown(context.Background()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/demo/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	seen := ""
	for !strings.Contains(seen, "Alpha User") {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("native initial frame: %v", err)
		}
		seen += string(data)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("\x1b[B")); err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("\r")); err != nil {
		t.Fatal(err)
	}
	seen = ""
	for !strings.Contains(seen, "Switched to beta@example.invalid") {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("native switch frame: %v, output=%q", err, seen)
		}
		seen += string(data)
		if len(seen) > 200000 {
			t.Fatalf("no native switch feedback: %q", seen[len(seen)-1000:])
		}
	}
	if err := conn.Write(ctx, websocket.MessageBinary, []byte("a")); err != nil {
		t.Fatal(err)
	}
	seen = ""
	for !strings.Contains(seen, "Unavailable in demo") {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("native denied action: %v, output=%q", err, seen)
		}
		seen += string(data)
	}
	second, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/demo/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})
	if err != nil {
		t.Fatal(err)
	}
	secondSeen := ""
	for !strings.Contains(secondSeen, "ACTIVE") || !strings.Contains(secondSeen, "Alpha User") {
		_, data, err := second.Read(ctx)
		if err != nil {
			t.Fatalf("isolated visitor: %v, output=%q", err, secondSeen)
		}
		secondSeen += string(data)
	}
	_ = second.Close(websocket.StatusNormalClosure, "done")
	_ = conn.Close(websocket.StatusNormalClosure, "done")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(root)
		if err == nil && len(entries) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("demo home remained after disconnect")
}
