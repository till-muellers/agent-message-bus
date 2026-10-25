package detach

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperEnv = "AMB_DETACH_HELPER=1"

// TestHelperProcess is the detached child: it logs readiness then sleeps until killed.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("AMB_DETACH_HELPER") != "1" {
		t.Skip("helper only")
	}
	fmt.Println("helper ready", strings.Join(os.Args[1:], " "))
	time.Sleep(time.Hour)
}

func TestStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath, pidPath := filepath.Join(dir, "x.log"), filepath.Join(dir, "x.pid")
	args := []string{"-test.run=^TestHelperProcess$", Flag}
	pid, err := Start(args, append(os.Environ(), helperEnv), logPath, pidPath)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("find %d: %v", pid, err)
	}
	t.Cleanup(func() { _ = proc.Kill() })
	if got, _ := os.ReadFile(pidPath); string(got) != strconv.Itoa(pid) {
		t.Fatalf("pid file %q, want %d", got, pid)
	}
	waitForLog(t, logPath, "helper ready -test.run=^TestHelperProcess$\n")
	if err := proc.Kill(); err != nil {
		t.Fatalf("child %d not alive: %v", pid, err)
	}
}

func waitForLog(t *testing.T, path, want string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if b, _ := os.ReadFile(path); strings.Contains(string(b), want) {
			return
		}
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("log %q never contained %q", b, want)
}

func TestStripFlag(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want []string }{
		{in: []string{}, want: []string{}},
		{in: []string{Flag}, want: []string{}},
		{in: []string{"a", Flag, "b", "-detached"}, want: []string{"a", "b"}},
	}
	for _, tc := range tests {
		if got := stripFlag(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("stripFlag(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
