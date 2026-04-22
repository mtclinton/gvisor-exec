# gvisor-exec

Run any binary inside a [gVisor](https://gvisor.dev) sandbox from the command
line. A single Go binary that wraps `runsc`, builds an ephemeral OCI bundle,
and tears it down on exit. Like `firejail`, but with gVisor's userspace-kernel
isolation instead of seccomp + namespaces.

```shell
$ gvisor-exec -- uname -a
Linux gvisor-exec 4.4.0 #1 SMP Sun Jan 10 15:06:54 PST 2016 x86_64 GNU/Linux
```

The `4.4.0` kernel version is gVisor's spoofed constant — your actual syscalls
never reach the host kernel. They're intercepted by gVisor's Sentry (a
userspace kernel written in Go) and re-implemented there.

## Why

- Running downloaded scripts, build tools, or CI artifacts without giving them
  host-kernel access.
- Isolating AI-generated code before executing it.
- Learning gVisor: the generated bundle is easy to inspect (`-keep-bundle`).
- No Docker daemon, no containerd, no root required.

## Install

Requires Go 1.24+ and `runsc` on the host. Install runsc from
<https://gvisor.dev/docs/user_guide/install/>.

```shell
go build -o gvisor-exec ./cmd/gvisor-exec
sudo install -m 0755 gvisor-exec /usr/local/bin/
```

Or just run `make build`.

## Usage

```
gvisor-exec [flags] -- <cmd> [args...]
```

Everything after `--` is the command to run inside the sandbox. Common flags:

| Flag | Default | What it does |
|---|---|---|
| `-platform` | `systrap` | Syscall interception mechanism: `systrap`, `ptrace`, or `kvm`. |
| `-network` | `none` | Network: `none`, `host`, or `sandbox` (needs veth + root). |
| `-cwd` | `$PWD` | Working directory inside the sandbox. |
| `-bind HOST[:DEST]` | (none) | Writable bind mount — writes are ephemeral (see below). |
| `-ro-bind HOST[:DEST]` | (none) | Read-only bind mount. |
| `-tmpfs DEST` | (none) | Extra tmpfs mount. `/tmp` is always mounted. |
| `-env KEY=VALUE` | | Set an environment variable (repeatable). |
| `-env-inherit KEY` | | Forward a host environment variable (repeatable). |
| `-uid` / `-gid` | `0` | User/group inside the sandbox — see "UIDs" below. |
| `-timeout` | `0` | Kill the sandbox after the duration. |
| `-keep-bundle` | `false` | Leave the bundle on disk for inspection. |
| `-verbose` | `false` | Print the runsc invocation and bundle path. |
| `-trace` | `false` | Enable `runsc --strace`. |

## Recipes

Five runnable scripts covering the common shapes live in
[`examples/`](examples/). Start with `make examples` (or
`./examples/run-all.sh`) to see them all. The table below is the short
version.

| Task | Command |
|---|---|
| Run a script piped via stdin | `cat script.sh \| gvisor-exec -- /bin/sh` |
| Run a script on disk | `gvisor-exec -- /bin/sh /home/me/script.sh` |
| Eval Python from stdin | `echo 'print(1+1)' \| gvisor-exec -- /usr/bin/python3` |
| Build with source bind-mounted | `gvisor-exec -bind "$PWD:/mnt" -cwd /mnt -- make` |
| Build and extract the artifact | `gvisor-exec -ro-bind "$PWD:/mnt" -cwd /tmp -- sh -c 'cp /mnt/*.c . && gcc *.c -o out && tar c out' \| tar x` |
| Inspect what a binary does | `gvisor-exec -- ./probe` (then read stdout/stderr) |
| See the isolation from inside | `gvisor-exec -- /bin/sh -c 'uname -r; id; ls /proc \| grep -cE "^[0-9]+$"'` |

### Things to know while writing recipes

1. **Scripts have to be visible in the sandbox.** The host root is the
   sandbox root, so anything under `/home`, `/opt`, `/usr` etc. works.
   `/tmp` is a fresh tmpfs — files written to `/tmp` on the *host* before
   invoking will not appear inside. Use a different path or pipe via stdin.
2. **Writes are ephemeral.** When the process exits, the overlay is torn
   down. To export bytes, pipe them to stdout (e.g., `tar c artifact`) and
   extract on the host.
3. **Bind destinations must already exist on the host.** `-bind "$PWD:/work"`
   fails unless `/work` exists. Use `/mnt`, `/opt`, or a path under your
   `$HOME`.
4. **Network is off by default** and `-network host` has limited routing
   under `runsc --rootless`. For real outbound access you need non-rootless
   runsc, which this tool doesn't set up for you today.

## Behavior

### Filesystem

- The **host root (`/`)** is the sandbox rootfs, exposed **read-only**.
  Applications see the host filesystem as if they mounted it `ro`.
- A **writable overlay** sits on top of the rootfs and every bind mount. The
  overlay is backed by a file under the bundle dir (mode `self`) or RAM
  (`-overlay memory`). Writes succeed but **do not persist** to the host.
- `/tmp`, `/proc`, `/sys`, `/dev` are always fresh sandbox-local mounts.

### Network

Default is `none`: the sandbox has only its own loopback. No veth, no bridge,
no packets leave the machine. `-network host` shares the host netns — fastest
but skips gVisor's netstack isolation. `-network sandbox` is the full
gVisor-netstack mode with a veth to a host bridge; needs CNI-style setup and
root, so rarely the right choice for one-off CLI use.

### UIDs

gvisor-exec runs `runsc --rootless`. Rootless runsc installs a
single-entry user namespace: *host uid 1000 ↔ sandbox uid 0*. Process owner
**inside** the sandbox defaults to `0:0`, which maps back to your host uid
outside. That's the mental model:

- Files you own on the host appear as `root:root` inside the sandbox. As
  sandbox root you can read/write them. Writes are captured by the overlay
  and thrown away.
- Files owned by host root appear as `nobody:nogroup` inside the sandbox.
  They are readable if their host mode allows (`/etc/passwd` is world-readable,
  so it works) and writable by no one — even though you're "root" inside, the
  userns won't let you escalate against unmapped host uids.

You can override with `-uid N -gid N` to run the sandbox process as an
arbitrary uid/gid (matching the userns mapping for uid 0 only; other uids see
unmapped files as nobody).

### Bind mounts

- `-ro-bind HOST[:DEST]` — source exposed read-only. Writes return `EROFS`.
- `-bind HOST[:DEST]` — source visible, writes succeed into the per-mount
  overlay but do not reach `HOST`. Use for programs that need to read *and*
  write their own files but shouldn't affect the host.
- Destinations must already exist on the host filesystem (because rootfs is
  `/` and the Gofer cannot mkdir into a read-only root). Common choices:
  `/mnt`, `/opt`, `/var/tmp`, anywhere under `/home/$USER`.

## Architecture

```
┌────────────────────────── host kernel ──────────────────────────┐
│                                                                 │
│   gvisor-exec (Go)                                              │
│     ├─ builds OCI bundle in /tmp/gvisor-exec-XXXX/              │
│     └─ execs runsc --rootless run                               │
│                                                                 │
│       runsc (go binary)                                         │
│         ├─ runsc-sandbox (the Sentry) — intercepts syscalls     │
│         │   └─ your binary runs here, syscall-trapped           │
│         └─ runsc-gofer — proxies filesystem I/O to host         │
└─────────────────────────────────────────────────────────────────┘
```

Inside the sandbox:
- **Sentry** handles most syscalls in Go, never calling the host kernel for
  them. It issues about 68 host syscalls of its own, all under a tight
  seccomp filter.
- **Gofer** is the only process allowed to open host files. It reads bytes
  for the Sentry over a Unix socket (LISAFS protocol).
- **Platform** (systrap, the default) uses seccomp `RET_TRAP` + shared memory
  to trap the guest's syscalls and route them to the Sentry.

gvisor-exec itself is a thin driver. It builds the OCI spec, invokes runsc,
waits on it, and cleans up. Total ~500 lines of Go.

## Building

```shell
make build         # gvisor-exec binary at ./gvisor-exec
make test          # unit + integration tests (integration needs runsc)
make unit          # unit tests only
make integration   # integration tests only
make smoke         # quick end-to-end check using the built binary
make examples      # run everything in examples/ sequentially
```

## Limitations

- Only tested on Linux amd64 with `runsc release-20260413.0`. gVisor's Go API
  is not stable for external consumers — this tool shells out to the `runsc`
  binary rather than importing gVisor packages.
- Writes to bind mounts are **ephemeral** by design — by the time you want
  persistent output, either pipe it to stdout or tar it up inside the
  sandbox and stream the archive.
- `runsc --rootless` needs `/dev/kvm` access only if you explicitly pick
  `-platform kvm`. The default `systrap` works on any 4.18+ kernel without
  special permissions.
- `-network sandbox` creates a veth and installs iptables NAT rules; that
  requires privileges a rootless user typically doesn't have. Use `-network
  host` for quick outbound connectivity, or leave the default `none`.

## Credits

Built on top of gVisor (Google).

## License

Licensed under the same terms as the other projects in this tree. See
`LICENSE` at the repository root if present.
