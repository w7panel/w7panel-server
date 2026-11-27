package procpath

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func RunInPidNamespace(pid string, script string) ([]byte, error) {
	return RunInPidNamespaceWithInput(pid, script, nil)
}

func RunInPidNamespaceWithInput(pid string, script string, stdin io.Reader) ([]byte, error) {
	var stdout bytes.Buffer
	err := runInPidNamespace(pid, script, stdin, &stdout)
	return stdout.Bytes(), err
}

func RunInPidNamespaceToWriter(pid string, script string, stdout io.Writer) error {
	return runInPidNamespace(pid, script, nil, stdout)
}

func runInPidNamespace(pid string, script string, stdin io.Reader, stdout io.Writer) error {
	if _, err := strconv.Atoi(pid); err != nil {
		return fmt.Errorf("invalid pid %q: %w", pid, err)
	}
	cmd := exec.Command("nsenter", "-t", pid, "--mount", "--uts", "--ipc", "--net", "--pid", "--", "sh", "-c", script)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func WriteFileInPidNamespace(pid, subPid, containerPath string, src io.Reader) error {
	target, err := InnerFilePath(subPid, containerPath)
	if err != nil {
		return err
	}
	script := "mkdir -p -- " + ShellQuote(filepath.Dir(target)) + " && cat > " + ShellQuote(target)
	_, err = RunInPidNamespaceWithInput(pid, script, src)
	return err
}

func CopyFileFromPidNamespace(pid, subPid, containerPath, localPath string) error {
	target, err := InnerFilePath(subPid, containerPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	err = RunInPidNamespaceToWriter(pid, "cat -- "+ShellQuote(target), dst)
	closeErr := dst.Close()
	if err != nil {
		_ = os.Remove(localPath)
		return err
	}
	return closeErr
}
