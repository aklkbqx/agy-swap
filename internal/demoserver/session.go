package demoserver

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

type session struct {
	cmd  *exec.Cmd
	ptmx *os.File
	home string
	once sync.Once
}

func startSession(ctx context.Context, binary, tempRoot string) (*session, error) {
	home, err := os.MkdirTemp(tempRoot, "agy-live-")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TERM=xterm-256color", "LANG=C.UTF-8"}
	cmd.Dir = home
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		_ = os.RemoveAll(home)
		return nil, err
	}
	return &session{cmd: cmd, ptmx: ptmx, home: home}, nil
}

func (s *session) resize(cols, rows int) error {
	return pty.Setsize(s.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (s *session) Close() {
	s.once.Do(func() {
		_ = s.ptmx.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
			_ = s.cmd.Wait()
		}
		_ = os.RemoveAll(s.home)
	})
}
