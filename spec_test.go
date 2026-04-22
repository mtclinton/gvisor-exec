package gvisorexec

import (
	"encoding/json"
	"strings"
	"testing"
)

func baseConfig(args ...string) Config {
	c := DefaultConfig()
	c.Args = args
	c.Cwd = "/"
	return c
}

func TestBuildSpec_Minimal(t *testing.T) {
	c := baseConfig("/bin/echo", "hi")

	raw, err := buildSpec(c)
	if err != nil {
		t.Fatalf("buildSpec: %v", err)
	}

	var s ociSpec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s.OCIVersion == "" {
		t.Error("ociVersion is empty")
	}
	if got, want := s.Process.Args, []string{"/bin/echo", "hi"}; !equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
	if !s.Root.Readonly {
		t.Error("root should be readonly")
	}
	if s.Root.Path != "/" {
		t.Errorf("root path = %q, want /", s.Root.Path)
	}
	if !s.Process.NoNewPrivileges {
		t.Error("noNewPrivileges should be true")
	}
	if len(s.Process.Capabilities.Bounding) != 0 {
		t.Errorf("bounding caps = %v, want empty", s.Process.Capabilities.Bounding)
	}

	wantNS := map[string]bool{"pid": true, "network": true, "ipc": true, "uts": true, "mount": true}
	gotNS := map[string]bool{}
	for _, ns := range s.Linux.Namespaces {
		gotNS[ns.Type] = true
	}
	for k := range wantNS {
		if !gotNS[k] {
			t.Errorf("missing namespace %q", k)
		}
	}

	wantDests := map[string]bool{"/proc": true, "/dev": true, "/sys": true, "/tmp": true}
	gotDests := map[string]bool{}
	for _, m := range s.Mounts {
		gotDests[m.Destination] = true
	}
	for d := range wantDests {
		if !gotDests[d] {
			t.Errorf("missing default mount %q", d)
		}
	}
}

func TestBuildSpec_EnvPreserved(t *testing.T) {
	c := baseConfig("/bin/true")
	c.Env = []string{"FOO=bar", "BAZ=qux"}

	raw, err := buildSpec(c)
	if err != nil {
		t.Fatal(err)
	}
	var s ociSpec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.Process.Env, ","); !strings.Contains(got, "FOO=bar") || !strings.Contains(got, "BAZ=qux") {
		t.Errorf("env = %v, want FOO=bar and BAZ=qux", s.Process.Env)
	}
}

func TestBuildSpec_ROBindMount(t *testing.T) {
	c := baseConfig("/bin/true")
	c.ROBindMounts = []Mount{{Source: "/usr", Destination: "/usr"}}

	raw, err := buildSpec(c)
	if err != nil {
		t.Fatal(err)
	}
	var s ociSpec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	var found *ociMount
	for i, m := range s.Mounts {
		if m.Destination == "/usr" {
			found = &s.Mounts[i]
		}
	}
	if found == nil {
		t.Fatal("bind mount /usr missing")
	}
	if found.Type != "bind" {
		t.Errorf("type = %q, want bind", found.Type)
	}
	hasRO := false
	for _, opt := range found.Options {
		if opt == "ro" {
			hasRO = true
		}
	}
	if !hasRO {
		t.Errorf("options = %v, want contains ro", found.Options)
	}
}

func TestBuildSpec_RWBindMount(t *testing.T) {
	c := baseConfig("/bin/true")
	c.BindMounts = []Mount{{Source: "/tmp", Destination: "/tmp"}}

	raw, err := buildSpec(c)
	if err != nil {
		t.Fatal(err)
	}
	var s ociSpec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	var found *ociMount
	for i, m := range s.Mounts {
		if m.Destination == "/tmp" && m.Type == "bind" {
			found = &s.Mounts[i]
		}
	}
	if found == nil {
		t.Fatal("rw bind mount missing")
	}
	for _, opt := range found.Options {
		if opt == "ro" {
			t.Errorf("rw mount should not be ro, got %v", found.Options)
		}
	}
}

func TestBuildSpec_TmpfsDeduplicated(t *testing.T) {
	c := baseConfig("/bin/true")
	c.Tmpfs = []string{"/tmp", "/scratch"}

	raw, err := buildSpec(c)
	if err != nil {
		t.Fatal(err)
	}
	var s ociSpec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	// /tmp is included once (as a default mount); /scratch should appear.
	tmpCount := 0
	foundScratch := false
	for _, m := range s.Mounts {
		if m.Destination == "/tmp" {
			tmpCount++
		}
		if m.Destination == "/scratch" {
			foundScratch = true
		}
	}
	if tmpCount != 1 {
		t.Errorf("/tmp appears %d times, want 1", tmpCount)
	}
	if !foundScratch {
		t.Error("/scratch tmpfs missing")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		mod  func(*Config)
		want string
	}{
		{"empty args", func(c *Config) { c.Args = nil }, "Args is empty"},
		{"invalid platform", func(c *Config) { c.Platform = "vfio" }, "invalid platform"},
		{"invalid network", func(c *Config) { c.Network = "cilium" }, "invalid network"},
		{"invalid overlay", func(c *Config) { c.Overlay = "disk" }, "invalid overlay"},
		{"relative rootfs", func(c *Config) { c.Rootfs = "relative" }, "rootfs must be absolute"},
		{"relative cwd", func(c *Config) { c.Cwd = "relative" }, "cwd must be absolute"},
		{"relative bind source", func(c *Config) {
			c.BindMounts = []Mount{{Source: "relative", Destination: "/x"}}
		}, "bind source must be absolute"},
		{"relative bind dest", func(c *Config) {
			c.BindMounts = []Mount{{Source: "/tmp", Destination: "relative"}}
		}, "bind destination must be absolute"},
		{"missing bind source", func(c *Config) {
			c.BindMounts = []Mount{{Source: "/does/not/exist/anywhere", Destination: "/tmp"}}
		}, "bind source"},
		{"missing bind dest on host rootfs", func(c *Config) {
			c.BindMounts = []Mount{{Source: "/tmp", Destination: "/does/not/exist/anywhere"}}
		}, "does not exist on host"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := baseConfig("/bin/true")
			tc.mod(&c)
			_, err := buildSpec(c)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want contains %q", err, tc.want)
			}
		})
	}
}

func TestOverlayArg(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "all:self"},
		{"self", "all:self"},
		{"memory", "all:memory"},
		{"none", "none"},
		{"dir=/foo", "all:dir=/foo"},
	}
	for _, tc := range tests {
		if got := overlayArg(tc.in); got != tc.want {
			t.Errorf("overlayArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildSpec_JSONRoundTrip(t *testing.T) {
	c := baseConfig("/bin/true")
	c.BindMounts = []Mount{{Source: "/tmp", Destination: "/tmp"}}
	c.ROBindMounts = []Mount{{Source: "/usr", Destination: "/usr"}}
	c.Tmpfs = []string{"/scratch"}
	c.Env = []string{"K=V"}

	raw, err := buildSpec(c)
	if err != nil {
		t.Fatal(err)
	}
	// Must round-trip through json.Unmarshal without errors.
	var anyJSON map[string]any
	if err := json.Unmarshal(raw, &anyJSON); err != nil {
		t.Fatalf("round trip unmarshal: %v", err)
	}
	if _, ok := anyJSON["process"].(map[string]any)["capabilities"]; !ok {
		t.Error("missing capabilities in spec")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
