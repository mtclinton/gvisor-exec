package gvisorexec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewBundle_WritesConfig(t *testing.T) {
	c := baseConfig("/bin/true")
	b, err := NewBundle(c)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	t.Cleanup(func() { _ = b.Cleanup() })

	info, err := os.Stat(b.Dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("bundle dir missing: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(b.Dir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var spec ociSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse config.json: %v", err)
	}
	if spec.Process.Args[0] != "/bin/true" {
		t.Errorf("spec args[0] = %q, want /bin/true", spec.Process.Args[0])
	}
	if !strings.HasPrefix(b.ID, "gve-") {
		t.Errorf("bundle id = %q, want gve- prefix", b.ID)
	}
}

func TestNewBundle_RejectsInvalidConfig(t *testing.T) {
	c := baseConfig()
	c.Args = nil
	if _, err := NewBundle(c); err == nil {
		t.Fatal("expected error for empty args, got nil")
	}
}

func TestBundle_Cleanup(t *testing.T) {
	c := baseConfig("/bin/true")
	b, err := NewBundle(c)
	if err != nil {
		t.Fatal(err)
	}
	dir := b.Dir
	if err := b.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("bundle dir %s still exists after cleanup", dir)
	}
}

func TestBundle_CleanupNilSafe(t *testing.T) {
	var b *Bundle
	if err := b.Cleanup(); err != nil {
		t.Errorf("nil Cleanup returned %v, want nil", err)
	}
	if err := (&Bundle{}).Cleanup(); err != nil {
		t.Errorf("zero Cleanup returned %v, want nil", err)
	}
}
