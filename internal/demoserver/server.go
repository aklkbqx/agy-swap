package demoserver

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Config struct {
	Origin      string
	Binary      string
	TempRoot    string
	MaxSessions int
	MaxPerIP    int
}

type Server struct {
	config Config
	mu     sync.Mutex
	active int
	perIP  map[string]int
}

func New(config Config) *Server {
	if config.MaxSessions <= 0 {
		config.MaxSessions = 8
	}
	if config.MaxPerIP <= 0 {
		config.MaxPerIP = 8
	}
	return &Server{config: config, perIP: make(map[string]int)}
}

func (s *Server) acquire(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active >= s.config.MaxSessions || s.perIP[ip] >= s.config.MaxPerIP {
		return false
	}
	s.active++
	s.perIP[ip]++
	return true
}

func (s *Server) release(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	s.perIP[ip]--
	if s.perIP[ip] == 0 {
		delete(s.perIP, ip)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/demo/health" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
		return
	}
	if r.URL.Path != "/demo/ws" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.config.Origin == "" || r.Header.Get("Origin") != s.config.Origin {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !s.acquire(ip) {
		http.Error(w, "demo is busy", http.StatusTooManyRequests)
		return
	}
	defer s.release(ip)
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{s.config.Origin}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(2048)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	terminal, err := startSession(ctx, s.config.Binary, s.config.TempRoot)
	if err != nil {
		_ = writeStatus(ctx, conn, "error", "Demo could not start")
		return
	}
	defer terminal.Close()
	if writeStatus(ctx, conn, "ready", "") != nil {
		return
	}
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		buffer := make([]byte, 8192)
		for {
			n, readErr := terminal.ptmx.Read(buffer)
			if n > 0 {
				writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				writeErr := conn.Write(writeCtx, websocket.MessageBinary, buffer[:n])
				stop()
				if writeErr != nil {
					cancel()
					return
				}
			}
			if readErr != nil {
				cancel()
				return
			}
		}
	}()
	defer func() { terminal.Close(); <-outputDone }()
	window := time.Now()
	messageCount := 0
	for {
		readCtx, stop := context.WithTimeout(ctx, 5*time.Minute)
		kind, data, readErr := conn.Read(readCtx)
		stop()
		if readErr != nil {
			return
		}
		if time.Since(window) >= time.Second {
			window, messageCount = time.Now(), 0
		}
		messageCount++
		if messageCount > 128 {
			_ = conn.Close(websocket.StatusPolicyViolation, "input rate exceeded")
			return
		}
		switch kind {
		case websocket.MessageBinary:
			if len(data) == 0 || len(data) > 1024 {
				return
			}
			if _, err := terminal.ptmx.Write(data); err != nil {
				return
			}
		case websocket.MessageText:
			cols, rows, err := parseResize(data)
			if err != nil || terminal.resize(cols, rows) != nil {
				return
			}
		default:
			return
		}
	}
}

func writeStatus(ctx context.Context, conn *websocket.Conn, kind, message string) error {
	data, _ := json.Marshal(map[string]string{"type": kind, "message": message})
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, data)
}
