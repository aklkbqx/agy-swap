package main

import "testing"

func TestListenAddrDefaultsToLoopback(t *testing.T) {
	t.Setenv("AGY_DEMO_ADDR", "")
	if got := listenAddr(); got != "127.0.0.1:8787" {
		t.Fatalf("default listen address = %q, want loopback 127.0.0.1:8787", got)
	}
	t.Setenv("AGY_DEMO_ADDR", "127.0.0.1:9000")
	if got := listenAddr(); got != "127.0.0.1:9000" {
		t.Fatalf("override = %q, want 127.0.0.1:9000", got)
	}
}
