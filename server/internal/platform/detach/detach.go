// Package detach launches the current executable as a background process.
package detach

import (
	"fmt"
	"os"
	"os/exec"
)

// Start launches os.Executable() with args and env, appending output to logPath.
// It releases the child and returns its pid.
func Start(args, env []string, logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locating executable: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return 0, fmt.Errorf("opening log %s: %w", logPath, err)
	}
	defer func() { _ = logFile.Close() }()
	cmd := exec.Command(exe, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, logFile, logFile
	cmd.SysProcAttr = sysProcAttr()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting %s: %w", exe, err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return 0, fmt.Errorf("releasing child %d: %w", pid, err)
	}
	return pid, nil
}
