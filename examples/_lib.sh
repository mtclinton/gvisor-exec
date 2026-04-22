#!/bin/sh
# Shared setup for the example scripts. Sourced by each example.
# Exits if gvisor-exec or a required tool is unavailable.

set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
GVE="$SCRIPT_DIR/../gvisor-exec"
if [ ! -x "$GVE" ]; then
    if command -v gvisor-exec >/dev/null 2>&1; then
        GVE=$(command -v gvisor-exec)
    else
        echo "gvisor-exec not found." >&2
        echo "Run 'make build' in the repo root, or install the binary on PATH." >&2
        exit 1
    fi
fi

# require_cmd bails out with a skip note if the named binary isn't installed.
require_cmd() {
    for c in "$@"; do
        if ! command -v "$c" >/dev/null 2>&1; then
            echo "skip: $c not installed on host" >&2
            exit 0
        fi
    done
}

banner() {
    echo "-- $* --"
}
