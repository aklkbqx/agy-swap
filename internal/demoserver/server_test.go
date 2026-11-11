package demoserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResizeRejectsUnknownAndClampsDimensions(t *testing.T) {
	if _, _, err := parseResize([]byte(`{"type":"key","cols":80,"rows":24}`)); err == nil {
		t.Fatal("unknown control type accepted")
	}
	if _, _, err := parseResize([]byte(`{"type":"resize","cols":80,"rows":24}{"type":"resize","cols":1,"rows":1}`)); err == nil {
		t.Fatal("trailing control frame accepted")
	}
	cols, rows, err := parseResize([]byte(`{"type":"resize","cols":999,"rows":1}`))
	if err != nil || cols != 180 || rows != 12 {
		t.Fatalf("clamped size = %d x %d, %v", cols, rows, err)
	}
	cols, rows, err = parseResize([]byte(`{"type":"resize","cols":28,"rows":12}`))
	if err != nil || cols != 28 || rows != 12 {
		t.Fatalf("smallest TUI size = %d x %d, %v; want 28 x 12", cols, rows, err)
	}
	cols, rows, err = parseResize([]byte(`{"type":"resize","cols":1,"rows":1}`))
	if err != nil || cols != 28 || rows != 12 {
		t.Fatalf("tiny size clamps to %d x %d, %v; want 28 x 12", cols, rows, err)
	}
}

func TestGatewayRejectsCrossOriginBeforeUpgrade(t *testing.T) {
	server := New(Config{Origin: "https://agy-swap.aklkbqx.com", Binary: "/unused"})
	req := httptest.NewRequest(http.MethodGet, "/demo/ws", nil)
	req.Header.Set("Origin", "https://outside.example")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGatewayRequiresWebSocketUpgrade(t *testing.T) {
	server := New(Config{Origin: "https://agy-swap.aklkbqx.com", Binary: "/unused"})
	req := httptest.NewRequest(http.MethodGet, "/demo/ws", nil)
	req.Header.Set("Origin", "https://agy-swap.aklkbqx.com")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestGatewayBoundsConcurrentSessions(t *testing.T) {
	server := New(Config{MaxSessions: 2, MaxPerIP: 1})
	if !server.acquire("visitor-a") || server.acquire("visitor-a") {
		t.Fatal("per-visitor limit was not enforced")
	}
	if !server.acquire("visitor-b") || server.acquire("visitor-c") {
		t.Fatal("global limit was not enforced")
	}
	server.release("visitor-a")
	if !server.acquire("visitor-c") {
		t.Fatal("slot was not released")
	}
}

func TestClientIPExtractsForwardedHeadersFromLoopbackProxy(t *testing.T) {
	server := New(Config{})
	if server.config.MaxPerIP != 2 {
		t.Fatalf("expected default MaxPerIP = 2 (< MaxSessions 8), got %d", server.config.MaxPerIP)
	}

	req := httptest.NewRequest(http.MethodGet, "/demo/ws", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 127.0.0.1")
	if got := clientIP(req); got != "203.0.113.10" {
		t.Fatalf("clientIP with loopback X-Forwarded-For = %q, want 203.0.113.10", got)
	}

	reqReal := httptest.NewRequest(http.MethodGet, "/demo/ws", nil)
	reqReal.RemoteAddr = "127.0.0.1:54321"
	reqReal.Header.Set("X-Real-IP", "198.51.100.7")
	if got := clientIP(reqReal); got != "198.51.100.7" {
		t.Fatalf("clientIP with loopback X-Real-IP = %q, want 198.51.100.7", got)
	}

	// Untrusted non-loopback peer cannot spoof X-Forwarded-For.
	reqSpoof := httptest.NewRequest(http.MethodGet, "/demo/ws", nil)
	reqSpoof.RemoteAddr = "198.51.100.99:54321"
	reqSpoof.Header.Set("X-Forwarded-For", "203.0.113.10")
	if got := clientIP(reqSpoof); got != "198.51.100.99" {
		t.Fatalf("clientIP from non-loopback peer trusted spoofed XFF: got %q, want 198.51.100.99", got)
	}
}
