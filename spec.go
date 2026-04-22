package gvisorexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// buildSpec generates the OCI runtime-spec JSON matching c.
func buildSpec(c Config) ([]byte, error) {
	if err := validate(c); err != nil {
		return nil, err
	}

	spec := ociSpec{
		OCIVersion: "1.0.0",
		Process: ociProcess{
			Terminal: false,
			User:     ociUser{UID: c.UID, GID: c.GID},
			Args:     c.Args,
			Env:      c.Env,
			Cwd:      c.Cwd,
			Capabilities: ociCapabilities{
				Bounding:    []string{},
				Effective:   []string{},
				Permitted:   []string{},
				Inheritable: []string{},
				Ambient:     []string{},
			},
			Rlimits:         []ociRlimit{{Type: "RLIMIT_NOFILE", Hard: 65536, Soft: 65536}},
			NoNewPrivileges: true,
		},
		Root:     ociRoot{Path: c.Rootfs, Readonly: true},
		Hostname: c.Hostname,
		Mounts: []ociMount{
			{Destination: "/proc", Type: "proc", Source: "proc"},
			{Destination: "/dev", Type: "tmpfs", Source: "tmpfs"},
			{Destination: "/sys", Type: "sysfs", Source: "sysfs", Options: []string{"nosuid", "noexec", "nodev", "ro"}},
			{Destination: "/tmp", Type: "tmpfs", Source: "tmpfs", Options: []string{"nosuid", "nodev", "mode=1777"}},
		},
		Linux: ociLinux{
			Namespaces: []ociNamespace{
				{Type: "pid"},
				{Type: "network"},
				{Type: "ipc"},
				{Type: "uts"},
				{Type: "mount"},
			},
		},
	}

	for _, m := range c.ROBindMounts {
		spec.Mounts = append(spec.Mounts, ociMount{
			Destination: m.Destination,
			Type:        "bind",
			Source:      m.Source,
			Options:     []string{"bind", "ro"},
		})
	}
	for _, m := range c.BindMounts {
		spec.Mounts = append(spec.Mounts, ociMount{
			Destination: m.Destination,
			Type:        "bind",
			Source:      m.Source,
			Options:     []string{"bind", "rw"},
		})
	}
	for _, dst := range c.Tmpfs {
		if dst == "/tmp" {
			continue
		}
		spec.Mounts = append(spec.Mounts, ociMount{
			Destination: dst,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "nodev", "mode=1777"},
		})
	}

	return json.MarshalIndent(spec, "", "  ")
}

func validate(c Config) error {
	if len(c.Args) == 0 {
		return errors.New("gvisorexec: config: Args is empty")
	}
	switch c.Platform {
	case "systrap", "ptrace", "kvm":
	default:
		return fmt.Errorf("gvisorexec: config: invalid platform %q", c.Platform)
	}
	switch c.Network {
	case "none", "host", "sandbox":
	default:
		return fmt.Errorf("gvisorexec: config: invalid network %q", c.Network)
	}
	switch c.Overlay {
	case "self", "memory", "none":
	default:
		return fmt.Errorf("gvisorexec: config: invalid overlay %q", c.Overlay)
	}
	if !filepath.IsAbs(c.Rootfs) {
		return fmt.Errorf("gvisorexec: config: rootfs must be absolute, got %q", c.Rootfs)
	}
	if c.Cwd != "" && !filepath.IsAbs(c.Cwd) {
		return fmt.Errorf("gvisorexec: config: cwd must be absolute, got %q", c.Cwd)
	}
	for _, m := range append(append([]Mount{}, c.BindMounts...), c.ROBindMounts...) {
		if !filepath.IsAbs(m.Source) {
			return fmt.Errorf("gvisorexec: config: bind source must be absolute, got %q", m.Source)
		}
		if !filepath.IsAbs(m.Destination) {
			return fmt.Errorf("gvisorexec: config: bind destination must be absolute, got %q", m.Destination)
		}
		if _, err := os.Stat(m.Source); err != nil {
			return fmt.Errorf("gvisorexec: config: bind source %q: %w", m.Source, err)
		}
		// The gofer does not have permission to mkdir into a read-only
		// host rootfs, so the destination must already exist when the
		// sandbox rootfs is the host root.
		if c.Rootfs == "/" {
			destOnHost := m.Destination
			if _, err := os.Stat(destOnHost); err != nil {
				return fmt.Errorf("gvisorexec: bind destination %q does not exist on host; the gofer cannot create mount points in a read-only host rootfs", m.Destination)
			}
		}
	}
	return nil
}

type ociSpec struct {
	OCIVersion string     `json:"ociVersion"`
	Process    ociProcess `json:"process"`
	Root       ociRoot    `json:"root"`
	Hostname   string     `json:"hostname"`
	Mounts     []ociMount `json:"mounts"`
	Linux      ociLinux   `json:"linux"`
}

type ociProcess struct {
	Terminal        bool            `json:"terminal"`
	User            ociUser         `json:"user"`
	Args            []string        `json:"args"`
	Env             []string        `json:"env"`
	Cwd             string          `json:"cwd"`
	Capabilities    ociCapabilities `json:"capabilities"`
	Rlimits         []ociRlimit     `json:"rlimits"`
	NoNewPrivileges bool            `json:"noNewPrivileges"`
}

type ociUser struct {
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

type ociCapabilities struct {
	Bounding    []string `json:"bounding"`
	Effective   []string `json:"effective"`
	Permitted   []string `json:"permitted"`
	Inheritable []string `json:"inheritable"`
	Ambient     []string `json:"ambient"`
}

type ociRlimit struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}

type ociRoot struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly"`
}

type ociMount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Source      string   `json:"source"`
	Options     []string `json:"options,omitempty"`
}

type ociLinux struct {
	Namespaces []ociNamespace `json:"namespaces"`
}

type ociNamespace struct {
	Type string `json:"type"`
}
