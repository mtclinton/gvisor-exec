// Command gvisor-exec runs any binary inside a gVisor sandbox.
//
//	gvisor-exec [flags] -- <cmd> [args...]
//
// The host filesystem is exposed read-only by default; writes land in an
// ephemeral tmpfs overlay that is discarded on exit. Network, pid, mount,
// ipc, and uts namespaces are all isolated. The process runs with no
// capabilities and the spoofed 4.4.0 kernel that gVisor presents.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gve "github.com/mtclinton/gvisor-exec"
)

func main() {
	os.Exit(realMain(os.Args[1:]))
}

func realMain(args []string) int {
	fs := flag.NewFlagSet("gvisor-exec", flag.ContinueOnError)
	fs.Usage = func() { usage(fs.Output()) }

	cfg := gve.DefaultConfig()

	var (
		envList    stringSlice
		bindList   stringSlice
		roBindList stringSlice
		tmpfsList  stringSlice
		inheritEnv stringSlice
		uid        = int(cfg.UID)
		gid        = int(cfg.GID)
		timeout    time.Duration
		help       bool
	)

	fs.StringVar(&cfg.Platform, "platform", cfg.Platform, "gVisor platform (systrap, ptrace, kvm)")
	fs.StringVar(&cfg.Network, "network", cfg.Network, "network mode (none, host, sandbox)")
	fs.StringVar(&cfg.Hostname, "hostname", cfg.Hostname, "sandbox UTS hostname")
	fs.StringVar(&cfg.Cwd, "cwd", cfg.Cwd, "working directory inside the sandbox")
	fs.StringVar(&cfg.Rootfs, "rootfs", cfg.Rootfs, "host path used as sandbox rootfs")
	fs.StringVar(&cfg.Overlay, "overlay", cfg.Overlay, "writable overlay (self, memory, none)")
	fs.StringVar(&cfg.RunscPath, "runsc", "", "path to runsc binary (default: PATH lookup)")
	fs.Var(&envList, "env", "KEY=VALUE environment variable (repeatable)")
	fs.Var(&inheritEnv, "env-inherit", "inherit named variable from host environment (repeatable)")
	fs.Var(&bindList, "bind", "writable bind mount HOST[:DEST] (repeatable)")
	fs.Var(&roBindList, "ro-bind", "read-only bind mount HOST[:DEST] (repeatable)")
	fs.Var(&tmpfsList, "tmpfs", "extra tmpfs mount at DEST (repeatable)")
	fs.IntVar(&uid, "uid", uid, "user ID inside the sandbox")
	fs.IntVar(&gid, "gid", gid, "group ID inside the sandbox")
	fs.DurationVar(&timeout, "timeout", 0, "kill sandbox after duration (0 = no limit)")
	fs.BoolVar(&cfg.KeepBundle, "keep-bundle", false, "keep bundle directory for inspection")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "print the runsc invocation and bundle path")
	fs.BoolVar(&cfg.Trace, "trace", false, "enable runsc --strace")
	fs.BoolVar(&cfg.Debug, "debug", false, "enable runsc --debug")
	fs.BoolVar(&help, "help", false, "print this help and exit")
	fs.BoolVar(&help, "h", false, "print this help and exit")

	positional, err := parseArgs(fs, args)
	if err != nil {
		if err == flag.ErrHelp || help {
			usage(fs.Output())
			return 0
		}
		fmt.Fprintf(os.Stderr, "gvisor-exec: %v\n", err)
		return 2
	}
	if help || len(positional) == 0 {
		usage(fs.Output())
		if len(positional) == 0 {
			return 2
		}
		return 0
	}

	cfg.UID = uint32(uid)
	cfg.GID = uint32(gid)
	cfg.Timeout = timeout
	cfg.Args = positional
	cfg.Env = buildEnv(envList, inheritEnv)

	binds, err := parseMounts(bindList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gvisor-exec: -bind: %v\n", err)
		return 2
	}
	roBinds, err := parseMounts(roBindList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gvisor-exec: -ro-bind: %v\n", err)
		return 2
	}
	cfg.BindMounts = binds
	cfg.ROBindMounts = roBinds
	cfg.Tmpfs = tmpfsList

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code, err := gve.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gvisor-exec: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// parseArgs walks the argument list, applying flags until it hits `--` or the
// first non-flag. Everything after is returned as the positional command.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = args[i+1:]
			args = args[:i]
			break
		}
	}
	if positional == nil {
		// No "--": still try to find a non-flag terminator.
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		positional = fs.Args()
		return positional, nil
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return append(fs.Args(), positional...), nil
}

// buildEnv assembles the sandbox environment from the default set plus
// explicit -env values plus -env-inherit pass-throughs.
func buildEnv(explicit, inherit []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, kv := range explicit {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			seen[kv[:eq]] = true
		}
		out = append(out, kv)
	}
	for _, k := range inherit {
		if seen[k] {
			continue
		}
		v, ok := os.LookupEnv(k)
		if !ok {
			continue
		}
		out = append(out, k+"="+v)
		seen[k] = true
	}
	// Add sensible defaults for any key the user didn't set.
	for _, def := range []struct{ k, v string }{
		{"PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		{"HOME", "/tmp"},
	} {
		if !seen[def.k] {
			out = append(out, def.k+"="+def.v)
		}
	}
	return out
}

// parseMounts turns a list of "HOST[:DEST]" strings into Mount records.
func parseMounts(entries []string) ([]gve.Mount, error) {
	var out []gve.Mount
	for _, e := range entries {
		parts := strings.SplitN(e, ":", 2)
		host := parts[0]
		dest := host
		if len(parts) == 2 {
			dest = parts[1]
		}
		if host == "" {
			return nil, fmt.Errorf("empty host path in %q", e)
		}
		out = append(out, gve.Mount{Source: host, Destination: dest})
	}
	return out, nil
}

type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

func usage(w interface{ Write(p []byte) (int, error) }) {
	fmt.Fprint(w, `Usage: gvisor-exec [flags] -- <cmd> [args...]

Run any binary inside a gVisor sandbox.

The host filesystem is exposed read-only by default; writes land in an
ephemeral tmpfs overlay that is discarded on exit. Network is off by default.

Flags:
  -platform string    gVisor platform: systrap, ptrace, kvm (default systrap)
  -network string     network mode: none, host, sandbox (default none)
  -hostname string    sandbox UTS hostname (default gvisor-exec)
  -cwd string         working directory inside the sandbox (default $PWD)
  -rootfs string      host path used as sandbox rootfs (default /)
  -overlay string     writable overlay: self, memory, none (default self)
  -env KEY=VALUE      set an environment variable (repeatable)
  -env-inherit KEY    inherit named variable from host env (repeatable)
  -bind HOST[:DEST]   writable bind mount (repeatable)
  -ro-bind HOST[:DEST]  read-only bind mount (repeatable)
  -tmpfs DEST         extra tmpfs mount (repeatable)
  -uid N              user ID inside the sandbox (default 0, mapped to host user)
  -gid N              group ID inside the sandbox (default 0, mapped to host group)
  -timeout DURATION   kill sandbox after duration (default: 0 = no limit)
  -runsc PATH         path to runsc binary (default: PATH lookup)
  -keep-bundle        keep bundle directory for inspection
  -verbose            print the runsc invocation and bundle path
  -trace              enable runsc --strace
  -debug              enable runsc --debug

Examples:
  gvisor-exec -- uname -a
  gvisor-exec -network=host -- curl https://example.com
  gvisor-exec -env-inherit HOME -- sh -c 'echo $HOME'
  gvisor-exec -bind "$PWD:/work" -cwd /work -- ./build.sh
`)
}
