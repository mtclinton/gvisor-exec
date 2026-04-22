#!/bin/sh
# Demonstrates that a destructive script run inside gvisor-exec cannot touch
# the host filesystem or reach the network. The script tries to wipe /etc and
# phone home; neither succeeds, and the host /etc is unchanged on exit.

. "$(dirname "$0")/_lib.sh"

banner "host /etc before: $(ls /etc | wc -l) entries"

"$GVE" -- /bin/sh <<'SANDBOX'
echo "hi from sketchy script"
rm -rf /etc 2>&1 | head -2
curl -sSL http://evil.example.com 2>&1 | head -1
echo "exit"
SANDBOX

banner "host /etc after: $(ls /etc | wc -l) entries"
echo "OK"
