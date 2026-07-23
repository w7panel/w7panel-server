package remotecommand

import (
	"context"
	"io"
	"log/slog"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"k8s.io/client-go/tools/remotecommand"
)

type LocalExecutor struct {
	cmd *exec.Cmd
}

func NewLocalExecutor(cmd *exec.Cmd) *LocalExecutor {
	return &LocalExecutor{
		cmd: cmd,
	}
}

func (e LocalExecutor) StreamWithContext(ctx context.Context, options remotecommand.StreamOptions) error {
	command := ""
	args := []string(nil)
	if e.cmd != nil {
		command = e.cmd.Path
		args = e.cmd.Args
	}

	tty, err := pty.Start(e.cmd)
	if err != nil {
		return err
	}
	defer tty.Close()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- e.cmd.Wait()
	}()

	if options.Tty && options.TerminalSizeQueue != nil {
		go func() {
			for {
				size := options.TerminalSizeQueue.Next()
				if size == nil {
					return
				}

				err := pty.Setsize(tty, &pty.Winsize{Rows: size.Height, Cols: size.Width})
				if err != nil {
					slog.Error("setsize error", "err", err)
				}
			}
		}()
	}
	go func() {
		for {
			buf := make([]byte, 32*1024)
			read, err := tty.Read(buf)
			if err != nil {
				slog.Error("tty read error", "err", err)
				return
			}

			_, err = options.Stdout.Write(buf[:read])
			if err != nil {
				slog.Error("stdout write error", "err", err)
				return
			}
		}
	}()
	go func() {
		_, err := io.Copy(tty, options.Stdin)
		if err != nil {
			slog.Error("stdin copy error", "err", err)
			return
		}
	}()

	select {
	case err := <-waitCh:
		return err
	case <-ctx.Done():
		if e.cmd.Process != nil {
			if err := e.cmd.Process.Kill(); err != nil {
				slog.Warn("kill local command failed", "command", command, "args", args, "pid", e.cmd.Process.Pid, "err", err)
			}
		}
		_ = tty.Close()
		select {
		case err := <-waitCh:
			if err != nil {
				slog.Debug("local command exited after context cancellation", "command", command, "args", args, "err", err)
			}
		case <-time.After(5 * time.Second):
			slog.Warn("timed out waiting for local command after kill", "command", command, "args", args)
		}
		return ctx.Err()
	}
}
