//go:build !windows

package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A terminal that takes output slower than the screen produces it must not
// hold back the keyboard: keys pressed during a refresh are handled as they
// arrive, and quitting does not wait for frames nobody will see.
func TestInteractiveLoopAnswersKeysWhileTheTerminalReadsSlowly(t *testing.T) {
	t.Setenv("AGY_SWAP_REDUCED_MOTION", "0")
	t.Setenv("COLORTERM", "truecolor")
	f := newCLIFixture(t, "one@example.com", "two@example.com", "three@example.com", "four@example.com", "five@example.com")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/loadCodeAssist":
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`))
		case "/retrieveUserQuotaSummary":
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-time.After(400 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.42,"resetTime":"2030-01-01T00:00:00Z"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	f.a.http.oauthURL = server.URL + "/oauth"
	f.a.http.cloudAPI = server.URL + "/"
	f.a.quota = NewQuotaService(f.a.http, f.a.store)
	f.a.quota.vault = f.a.vault
	f.a.p = makePalette(true)
	f.a.color = true

	master, slave := openPTY(t)
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 50, Col: 180}); err != nil {
		t.Fatal(err)
	}
	f.a.In, f.a.Out = slave, slave

	// The terminal: it reads 100 KB a second, far less than a moving screen sends.
	const bytesPerSecond = 100_000
	go func() {
		buf := make([]byte, 4096)
		began, taken := time.Now(), 0
		for {
			for float64(taken) > time.Since(began).Seconds()*bytesPerSecond {
				time.Sleep(time.Millisecond)
			}
			n, err := master.Read(buf)
			taken += n
			if err != nil {
				return
			}
		}
	}()

	done := make(chan int, 1)
	go func() { done <- f.a.cmdInteractive(context.Background()) }()
	time.Sleep(300 * time.Millisecond)
	_, _ = master.WriteString("r")
	for i := 0; i < 100; i++ { // two seconds of held navigation keys during the refresh
		key := "j"
		if i%2 == 1 {
			key = "k"
		}
		_, _ = master.WriteString(key)
		time.Sleep(20 * time.Millisecond)
	}
	pressed := time.Now()
	_, _ = master.WriteString("q")
	select {
	case <-done:
		if waited := time.Since(pressed); waited > 1500*time.Millisecond {
			t.Fatalf("quit took %s after q on a slow terminal; keys queued behind terminal writes", waited.Round(time.Millisecond))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the screen never quit: keys are stuck behind terminal writes")
	}
}
