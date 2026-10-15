// Package collector enumerates disks and gathers SMART data by invoking the
// bundled smartctl/nvme binaries (or reading canned output in mock mode).
package collector

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner abstracts command execution so the collector can run against real
// binaries on-device or canned fixtures in dev.
type Runner interface {
	// Run executes argv and returns combined stdout. stderr is captured for
	// error context but SMART data is expected on stdout.
	Run(ctx context.Context, argv ...string) (stdout string, stderr string, err error)
}

// ExecRunner runs real commands with a timeout.
type ExecRunner struct{ Timeout time.Duration }

// Run implements Runner.
func (r ExecRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	if len(argv) == 0 {
		return "", "", fmt.Errorf("empty argv")
	}
	to := r.Timeout
	if to <= 0 {
		to = 15 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// MockRunner reads canned output from files under Dir, keyed by a sanitized
// command line. If no fixture exists it returns an error, letting the
// collector treat that command as unavailable (just like a missing binary).
type MockRunner struct{ Dir string }

// Run implements Runner.
func (m MockRunner) Run(ctx context.Context, argv ...string) (string, string, error) {
	key := mockKey(argv...)
	path := filepath.Join(m.Dir, key)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("mock fixture missing for %q: %w", key, err)
	}
	return string(b), "", nil
}

// mockKey turns a command line into a stable, filesystem-safe fixture name.
// e.g. ["smartctl","-a","-j","/dev/sda"] -> "smartctl_-a_-j__dev_sda.txt"
func mockKey(argv ...string) string {
	joined := strings.Join(argv, "_")
	var b strings.Builder
	for _, r := range joined {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.', r == '_':
			b.WriteRune(r)
		case r == '/':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	return b.String() + ".txt"
}
