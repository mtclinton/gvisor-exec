# gvisor-exec examples

Five self-contained scripts that demonstrate how `gvisor-exec` behaves on real
workloads. Each script is runnable on its own; `run-all.sh` runs the full
sequence.

## Prerequisites

- `gvisor-exec` binary built at the repo root (`make build`) or installed
  on `PATH`.
- `runsc` on `PATH`.
- `gcc`, `python3`, `curl`, `tar` (the scripts skip themselves with a note
  when an optional tool is missing).

## Running

```shell
# one-shot:
./examples/run-all.sh

# or individually:
./examples/01-untrusted-script.sh
./examples/02-python-eval.sh
./examples/03-compile-ephemeral.sh
./examples/04-compile-extract.sh
./examples/05-inspect-sandbox.sh
```

From the repo root: `make examples`.

## What each one shows

| Script | Demonstrates |
|---|---|
| `01-untrusted-script.sh` | A destructive shell script piped via stdin can't rm `/etc` and can't reach the network. Host `/etc` is unchanged before and after. |
| `02-python-eval.sh` | Python code from stdin runs normally; network calls fail; writes to `/tmp` succeed but vanish at exit. |
| `03-compile-ephemeral.sh` | A C source directory bind-mounted into the sandbox compiles + runs, but the host directory is unchanged afterwards (overlay discards the build artifacts). |
| `04-compile-extract.sh` | Same as 03, but the compiled binary is `tar`'d to stdout and extracted on the host — the pattern for getting bytes *out* of an ephemeral sandbox. |
| `05-inspect-sandbox.sh` | Prints what isolation looks like from inside: spoofed `4.4.0` kernel, PID 1, zero effective capabilities, empty `/sys/class/net`, read-only `/etc`. |

## Expected output (abridged)

Running `run-all.sh` prints something like:

```
== 01-untrusted-script.sh
-- host /etc before: 228 entries --
hi from sketchy script
rm: cannot remove '/etc/libaudit.conf': Permission denied
rm: cannot remove '/etc/vim/vimrc.tiny': Permission denied
curl: (6) Could not resolve host: evil.example.com
exit
-- host /etc after: 228 entries --
OK

== 05-inspect-sandbox.sh
kernel:        4.4.0  (gVisor spoofs 4.4.0 regardless of host)
hostname:      gvisor-exec
pid:           1  (fresh pid namespace)
uid/gid:       0/0
caps:          0000000000000000  (expect all zeros)
visible pids:  4  (host has hundreds)
net ifaces:
/etc writable: no
OK
```
