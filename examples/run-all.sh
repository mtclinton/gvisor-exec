#!/bin/sh
# Run every numbered example in order. Each script exits 0 on success.

set -eu
cd "$(dirname "$0")"

for ex in [0-9]*.sh; do
    echo
    echo "=========================================================="
    echo "== $ex"
    echo "=========================================================="
    sh "./$ex"
done
