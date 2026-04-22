package gvisorexec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireRunsc skips t if runsc is not installed.
func requireRunsc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("runsc"); err != nil {
		t.Skip("runsc not installed; skipping integration test")
	}
}

// sandbox runs cmd inside a default-configured sandbox and returns stdout,
// stderr, and the exit code. It fails the test on setup errors.
func sandbox(t *testing.T, cmd ...string) (string, string, int) {
	t.Helper()
	return sandboxWith(t, DefaultConfig(), cmd...)
}

func sandboxWith(t *testing.T, c Config, cmd ...string) (string, string, int) {
	t.Helper()
	c.Args = cmd
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code, err := Run(ctx, c, WithStdio(nil, &stdout, &stderr))
	if err != nil {
		t.Fatalf("Run: %v\nstderr:\n%s", err, stderr.String())
	}
	return stdout.String(), stderr.String(), code
}

func TestIntegration_Hello(t *testing.T) {
	requireRunsc(t)
	stdout, _, code := sandbox(t, "/bin/echo", "hello sandbox")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "hello sandbox") {
		t.Errorf("stdout = %q, want contains %q", stdout, "hello sandbox")
	}
}

func TestIntegration_ExitCode(t *testing.T) {
	requireRunsc(t)
	_, _, code := sandbox(t, "/bin/sh", "-c", "exit 42")
	if code != 42 {
		t.Errorf("exit = %d, want 42", code)
	}
}

func TestIntegration_SpoofedKernel(t *testing.T) {
	requireRunsc(t)
	stdout, _, _ := sandbox(t, "/bin/uname", "-r")
	if !strings.HasPrefix(strings.TrimSpace(stdout), "4.4.0") {
		t.Errorf("uname -r = %q, want gVisor's spoofed 4.4.0 prefix", stdout)
	}
}

func TestIntegration_SandboxHostname(t *testing.T) {
	requireRunsc(t)
	c := DefaultConfig()
	c.Hostname = "my-sandbox"
	stdout, _, _ := sandboxWith(t, c, "/bin/hostname")
	if strings.TrimSpace(stdout) != "my-sandbox" {
		t.Errorf("hostname = %q, want my-sandbox", stdout)
	}
}

func TestIntegration_PID1(t *testing.T) {
	requireRunsc(t)
	// The process sees itself as PID 1 because runsc gives it a fresh
	// pid namespace.
	stdout, _, _ := sandbox(t, "/bin/sh", "-c", "echo $$")
	if strings.TrimSpace(stdout) != "1" {
		t.Errorf("pid = %q, want 1 (from isolated pid namespace)", stdout)
	}
}

func TestIntegration_NetworkIsolated(t *testing.T) {
	requireRunsc(t)
	stdout, _, _ := sandbox(t, "/bin/sh", "-c", "cat /sys/class/net/*/address 2>/dev/null | wc -l")
	// Only the loopback's MAC address should be visible (0 since lo doesn't
	// expose a MAC on some kernels; at worst a single interface).
	n := strings.TrimSpace(stdout)
	if n != "0" && n != "1" {
		t.Errorf("visible NICs = %q, want 0 or 1 (loopback only)", n)
	}
}

func TestIntegration_EtcReadonly(t *testing.T) {
	requireRunsc(t)
	// Writing into /etc must fail: /etc is owned by host root and our
	// rootless sandbox user cannot escalate.
	_, _, code := sandbox(t, "/bin/sh", "-c", "touch /etc/gvisor-exec-should-fail 2>/dev/null")
	if code == 0 {
		t.Error("writing to /etc should fail inside sandbox")
	}
	if _, err := os.Stat("/etc/gvisor-exec-should-fail"); err == nil {
		t.Error("host /etc was modified by sandbox — isolation broken")
		_ = os.Remove("/etc/gvisor-exec-should-fail")
	}
}

func TestIntegration_TmpfsWritable(t *testing.T) {
	requireRunsc(t)
	stdout, _, code := sandbox(t, "/bin/sh", "-c", "echo written > /tmp/x && cat /tmp/x")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "written") {
		t.Errorf("stdout = %q, want 'written'", stdout)
	}
	// Ephemeral — host /tmp/x must not exist.
	if _, err := os.Stat("/tmp/x"); err == nil {
		t.Error("host /tmp/x exists; sandbox leaked write to host")
		_ = os.Remove("/tmp/x")
	}
}

func TestIntegration_CapabilitiesDropped(t *testing.T) {
	requireRunsc(t)
	stdout, _, _ := sandbox(t, "/bin/sh", "-c", "grep '^CapEff' /proc/self/status")
	// Expect zero effective caps.
	if !strings.Contains(stdout, "0000000000000000") {
		t.Errorf("CapEff = %q, want all zeros", stdout)
	}
}

func TestIntegration_UIDGIDApplied(t *testing.T) {
	requireRunsc(t)
	c := DefaultConfig()
	c.UID, c.GID = 1234, 5678
	stdout, _, _ := sandboxWith(t, c, "/bin/sh", "-c", "id -u; id -g")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 || lines[0] != "1234" || lines[1] != "5678" {
		t.Errorf("id output = %q, want uid 1234 gid 5678", stdout)
	}
}

func TestIntegration_EnvPassed(t *testing.T) {
	requireRunsc(t)
	c := DefaultConfig()
	c.Env = append([]string{"GVE_PROBE=probe-ok"}, c.Env...)
	stdout, _, _ := sandboxWith(t, c, "/bin/sh", "-c", "echo $GVE_PROBE")
	if strings.TrimSpace(stdout) != "probe-ok" {
		t.Errorf("env var not passed: got %q", stdout)
	}
}

func TestIntegration_ROBindMount(t *testing.T) {
	requireRunsc(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("read-only content"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := DefaultConfig()
	c.ROBindMounts = []Mount{{Source: dir, Destination: "/opt"}}
	stdout, _, code := sandboxWith(t, c, "/bin/sh", "-c", "cat /opt/note.txt; echo x > /opt/write 2>/dev/null; echo write-exit=$?")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "read-only content") {
		t.Errorf("contents not read: %q", stdout)
	}
	if !strings.Contains(stdout, "write-exit=") || strings.Contains(stdout, "write-exit=0") {
		t.Errorf("write should fail but didn't: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "write")); err == nil {
		t.Error("host file created despite ro mount")
	}
}

func TestIntegration_RWBindMountEphemeral(t *testing.T) {
	requireRunsc(t)
	dir := t.TempDir()
	// t.TempDir() is 0o700; chmod so the rootless userns-mapped sandbox
	// user can access it through the overlay (host uid maps to sandbox 0,
	// our process runs as host uid → a non-owner inside the mapping).
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := DefaultConfig()
	c.BindMounts = []Mount{{Source: dir, Destination: "/mnt"}}
	stdout, stderr, code := sandboxWith(t, c, "/bin/sh", "-c", "echo inside > /mnt/file && cat /mnt/file")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "inside") {
		t.Errorf("write+read failed inside sandbox: %q", stdout)
	}
	// Ephemeral: host source dir remains empty because the overlay
	// captures writes.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("host source dir has entries %v; writes should be ephemeral", entries)
	}
}

func TestIntegration_Timeout(t *testing.T) {
	requireRunsc(t)
	c := DefaultConfig()
	c.Timeout = 500 * time.Millisecond
	c.Args = []string{"/bin/sh", "-c", "sleep 30"}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	code, _ := Run(ctx, c, WithStdio(nil, nil, &stderr))
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("timeout did not fire in 500ms; elapsed=%v", elapsed)
	}
	if code == 0 {
		t.Errorf("exit = 0, want non-zero for timed-out sandbox; stderr=%s", stderr.String())
	}
}

func TestIntegration_RejectsMissingBindSource(t *testing.T) {
	requireRunsc(t)
	c := DefaultConfig()
	c.BindMounts = []Mount{{Source: "/this/path/really/does/not/exist", Destination: "/tmp"}}
	c.Args = []string{"/bin/true"}
	_, err := Run(context.Background(), c)
	if err == nil {
		t.Error("expected error for missing bind source")
	}
	if !strings.Contains(err.Error(), "bind source") {
		t.Errorf("err = %v, want contains 'bind source'", err)
	}
}

func TestIntegration_NoHostPIDsVisible(t *testing.T) {
	requireRunsc(t)
	stdout, _, _ := sandbox(t, "/bin/sh", "-c", "ls /proc | grep -c '^[0-9]*$'")
	n := strings.TrimSpace(stdout)
	// The sandbox has its own pid namespace — only sandbox processes
	// appear. Expect a small number (pid 1 for sh, maybe a child for ls).
	if n == "" {
		t.Fatal("no output from pid count")
	}
	// Host has hundreds of processes — anything >10 would suggest leak.
	if len(n) > 2 {
		t.Errorf("too many PIDs visible: %s (host PIDs may be leaking)", n)
	}
}
