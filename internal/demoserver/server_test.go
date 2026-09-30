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
