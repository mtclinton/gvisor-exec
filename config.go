// Package gvisorexec runs any binary inside a gVisor sandbox via runsc.
//
// The package turns a Config into an OCI bundle in a temp directory, invokes
// runsc against it, and cleans up. It is the engine behind the gvisor-exec
// CLI and can also be used as a library.
package gvisorexec

import (
	"os"
	"time"
)

// Config describes a single sandboxed execution.
type Config struct {
	// Args is the command to run; Args[0] is the program path.
	Args []string

	// Platform is the gVisor platform: "systrap", "ptrace", or "kvm".
	Platform string

	// Network is the gVisor network mode: "none", "host", or "sandbox".
	Network string

	// Hostname is the UTS hostname inside the sandbox.
	Hostname string

	// Cwd is the working directory inside the sandbox.
	Cwd string

	// Env is the full environment passed to the sandboxed process, as
	// KEY=VALUE strings. Nothing is inherited from the host unless the
	// caller explicitly includes it.
	Env []string

	// BindMounts are writable host-to-sandbox bind mounts.
	BindMounts []Mount

	// ROBindMounts are read-only host-to-sandbox bind mounts.
	ROBindMounts []Mount

	// Tmpfs is a list of extra tmpfs mount destinations. "/tmp" is always
	// mounted regardless of this field.
	Tmpfs []string

	// Rootfs is the absolute host path used as the sandbox rootfs. The
	// default, "/", exposes the host filesystem read-only.
	Rootfs string

	// UID and GID are the user and group IDs inside the sandbox.
	UID uint32
	GID uint32

	// Timeout kills the sandbox after the given duration. Zero means no
	// limit.
	Timeout time.Duration

	// Overlay controls the ephemeral overlay that captures writes:
	// "self" (file-backed tmpfs), "memory" (in-memory), or "none".
	// With "none" the rootfs is strictly read-only and writes to bind
	// mounts currently fail with EINVAL under runsc rootless mode. The
	// default "self" applies the overlay to all mounts so that writes
	// succeed everywhere but do not persist to the host.
	Overlay string

	// KeepBundle skips bundle cleanup; useful for debugging.
	KeepBundle bool

	// Verbose prints the runsc command and bundle path to stderr.
	Verbose bool

	// Trace enables runsc --strace.
	Trace bool

	// Debug enables runsc --debug.
	Debug bool

	// RunscPath is the path to the runsc binary. Empty searches PATH.
	RunscPath string
}

// Mount describes a single bind mount from a host path to a sandbox path.
type Mount struct {
	Source      string
	Destination string
}

// DefaultConfig returns a Config with sensible defaults: systrap platform, no
// network, writable tmpfs overlay, all capabilities dropped. The caller is
// expected to fill in Args and optionally override other fields.
//
// UID and GID default to 0: under runsc --rootless the sandbox's user
// namespace maps host uid → sandbox 0, so running as sandbox-0 is identity
// with the host user. This matches the standard container mental model and
// makes writes to user-owned bind-mounted directories work as expected.
func DefaultConfig() Config {
	cwd, _ := os.Getwd()
	return Config{
		Platform: "systrap",
		Network:  "none",
		Hostname: "gvisor-exec",
		Cwd:      cwd,
		Rootfs:   "/",
		UID:      0,
		GID:      0,
		Overlay:  "self",
		Env: []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=/tmp",
		},
	}
}
