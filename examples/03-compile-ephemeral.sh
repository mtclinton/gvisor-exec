#!/bin/sh
# Compile a C program inside the sandbox with the source bind-mounted
# read-write. The compile succeeds inside the overlay; the host directory is
# unchanged afterwards because the overlay is torn down.

. "$(dirname "$0")/_lib.sh"
require_cmd gcc

DEMO=$(mktemp -d)
trap 'rm -rf "$DEMO"' EXIT
chmod 755 "$DEMO"
cat > "$DEMO/hello.c" <<'EOF'
#include <stdio.h>
int main(void) { puts("compiled and ran inside gvisor-exec"); return 0; }
EOF

BEFORE=$(ls "$DEMO" | tr '\n' ' ')

"$GVE" -bind "$DEMO:/mnt" -cwd /mnt -- /bin/sh -c 'gcc hello.c -o hello && ./hello && echo "sandbox /mnt contents:" && ls'

AFTER=$(ls "$DEMO" | tr '\n' ' ')

banner "host dir contents"
echo "  before: $BEFORE"
echo "  after:  $AFTER"

if [ "$BEFORE" = "$AFTER" ]; then
    echo "OK"
else
    echo "ERROR: host dir was modified" >&2
    exit 1
fi
