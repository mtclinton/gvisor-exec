#!/bin/sh
# Print what the sandbox looks like from inside so you can see the isolation
# boundaries concretely: gVisor's spoofed kernel version, the fresh pid
# namespace (we're PID 1), dropped capabilities, and an empty network.

. "$(dirname "$0")/_lib.sh"

"$GVE" -- /bin/sh -c '
    printf "kernel:        %s  (gVisor spoofs 4.4.0 regardless of host)\n" "$(uname -r)"
    printf "hostname:      %s\n" "$(hostname)"
    printf "pid:           %s  (fresh pid namespace)\n" "$$"
    printf "uid/gid:       %s/%s\n" "$(id -u)" "$(id -g)"
    printf "caps:          %s  (expect all zeros)\n" "$(grep CapEff /proc/self/status | awk "{print \$2}")"
    printf "visible pids:  %s  (host has hundreds)\n" "$(ls /proc | grep -cE "^[0-9]+$")"
    printf "net ifaces:    %s\n" "$(ls /sys/class/net 2>/dev/null | tr "\n" " " || echo "(none visible)")"
    printf "/etc writable: "
    if touch /etc/gvisor-exec-probe 2>/dev/null; then
        echo "yes (BAD)"
    else
        echo "no"
    fi
'

echo "OK"
