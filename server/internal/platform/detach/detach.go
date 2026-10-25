// Package detach re-launches the current executable as a background process.
package detach

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// Flag is the command-line flag that requests detaching; it is stripped from the child's args.
const Flag = "--detached"

// Start launches os.Executable() with args (minus Flag) and env, appending output to logPath.
// It writes the child's pid to pidPath, releases the child and returns its pid.
func Start(args, env []string, logPath, pidPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("locating executable: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return 0, fmt.Errorf("opening log %s: %w", logPath, err)
	}
	defer func() { _ = logFile.Close() }()
	cmd := exec.Command(exe, stripFlag(args)...)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, logFile, logFile
	cmd.SysProcAttr = sysProcAttr()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting %s: %w", exe, err)
	}
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0o640); err != nil {
		_ = cmd.Process.Kill()
		return 0, fmt.Errorf("writing pid file %s: %w", pidPath, err)
	}
	if err := cmd.Process.Release(); err != nil {
		return 0, fmt.Errorf("releasing child %d: %w", pid, err)
	}
	return pid, nil
}

func stripFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a != Flag && a != "-detached" {
			out = append(out, a)
		}
	}
	return out
}
