#!/bin/sh
# Pipe a Python program via stdin and run it inside gvisor-exec. Network
# attempts fail; /tmp writes succeed but are discarded when the sandbox exits.

. "$(dirname "$0")/_lib.sh"
require_cmd python3

"$GVE" -- /usr/bin/python3 <<'PY'
import os, socket
print("running as pid:", os.getpid())
print("kernel:         ", os.uname().release, "(spoofed by gVisor)")

try:
    socket.socket().connect(("1.1.1.1", 80))
    print("net: reachable (unexpected)")
except OSError as e:
    print("net blocked:   ", e)

with open("/tmp/ephemeral", "w") as f:
    f.write("data")
print("wrote /tmp/ephemeral; it vanishes when the sandbox exits")
PY

echo "OK"
