package gvisorexec

import (
	"fmt"
	"os"
	"path/filepath"
)

// Bundle is an on-disk OCI bundle directory containing a config.json ready
// for runsc to run.
type Bundle struct {
	// Dir is the absolute path to the bundle directory.
	Dir string
	// ID is the container ID runsc will use.
	ID string
}

// NewBundle creates a fresh bundle in a new temp directory and writes the
// config.json derived from c. The caller owns the returned bundle and should
// call Cleanup when the container has exited.
func NewBundle(c Config) (*Bundle, error) {
	spec, err := buildSpec(c)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "gvisor-exec-*")
	if err != nil {
		return nil, fmt.Errorf("gvisorexec: create bundle dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.json"), spec, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("gvisorexec: write config.json: %w", err)
	}

	return &Bundle{
		Dir: dir,
		ID:  "gve-" + filepath.Base(dir),
	}, nil
}

// Cleanup removes the bundle directory.
func (b *Bundle) Cleanup() error {
	if b == nil || b.Dir == "" {
		return nil
	}
	return os.RemoveAll(b.Dir)
}
