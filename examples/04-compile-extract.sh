#!/bin/sh
# Variant of 03 that captures the build artifact by tar'ing it to stdout
# and extracting it on the host. This is the general pattern for getting
# bytes out of the sandbox when the overlay would otherwise discard them.

. "$(dirname "$0")/_lib.sh"
require_cmd gcc tar

DEMO=$(mktemp -d)
trap 'rm -rf "$DEMO"' EXIT
chmod 755 "$DEMO"
cat > "$DEMO/hello.c" <<'EOF'
#include <stdio.h>
int main(void) { puts("hello from the extracted binary"); return 0; }
EOF

"$GVE" -ro-bind "$DEMO:/mnt" -cwd /tmp -- /bin/sh -c '
    cp /mnt/*.c . &&
    gcc hello.c -o hello &&
    tar c hello
' | tar x -C "$DEMO"

banner "host dir now has the compiled binary"
ls -la "$DEMO"

banner "running it on the host"
"$DEMO/hello"

echo "OK"
