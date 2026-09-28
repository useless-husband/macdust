#!/bin/sh
# Cross-compile release binaries into dist/ (same as `make release`).
set -eu
cd "$(dirname "$0")/.."
exec make release
