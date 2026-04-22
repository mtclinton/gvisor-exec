package gvisorexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Run turns c into a bundle, runs it under runsc, and returns the exit code of
// the sandboxed process. Stdin, stdout, and stderr default to the host's
// standard streams; callers may override via opts.
func Run(ctx context.Context, c Config, opts ...RunOption) (int, error) {
	ro := runOptions{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
	for _, opt := range opts {
		opt(&ro)
	}

	bundle, err := NewBundle(c)
	if err != nil {
		return -1, err
	}
	if !c.KeepBundle {
		defer bundle.Cleanup()
	}

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "gvisor-exec: bundle=%s id=%s\n", bundle.Dir, bundle.ID)
	}

	runsc := c.RunscPath
	if runsc == "" {
		runsc = "runsc"
	}

	args := []string{
		"--rootless",
		"--ignore-cgroups",
		"--network=" + c.Network,
		"--platform=" + c.Platform,
		"--overlay2=" + overlayArg(c.Overlay),
	}
	if c.Debug {
		args = append(args, "--debug")
	}
	if c.Trace {
		args = append(args, "--strace")
	}
	args = append(args, "run", "--bundle", bundle.Dir, bundle.ID)

	runCtx := ctx
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, runsc, args...)
	cmd.Stdin = ro.stdin
	cmd.Stdout = ro.stdout
	cmd.Stderr = ro.stderr

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "gvisor-exec: exec %s %v\n", runsc, args)
	}

	err = cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("gvisorexec: run runsc: %w", err)
	}
	return 0, nil
}

// overlayArg turns a Config.Overlay value into the runsc --overlay2 argument.
// The "all" scope is used so that writes to bind mounts also land in the
// ephemeral overlay (runsc with "root:<medium>" rejects writes to bind mounts
// with EINVAL, a gVisor quirk worth steering around).
func overlayArg(mode string) string {
	switch mode {
	case "none":
		return "none"
	case "", "self":
		return "all:self"
	case "memory":
		return "all:memory"
	default:
		return "all:" + mode
	}
}

// RunOption configures Run. Use WithStdio to redirect the sandboxed process's
// standard streams.
type RunOption func(*runOptions)

type runOptions struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

// WithStdio overrides the stdin, stdout, and stderr streams connected to the
// sandboxed process.
func WithStdio(stdin io.Reader, stdout, stderr io.Writer) RunOption {
	return func(o *runOptions) {
		o.stdin = stdin
		o.stdout = stdout
		o.stderr = stderr
	}
}
