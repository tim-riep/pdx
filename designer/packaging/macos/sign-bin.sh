#!/bin/sh
# Signs command line binaries (pdx, pdx-server) with Developer ID and
# Hardened Runtime and notarizes them together. Bare binaries cannot carry
# a stapled ticket, Gatekeeper checks the notarization online.
#
#   packaging/macos/sign-bin.sh <binary>...
set -eu
. "$(dirname "$0")/lib.sh"
[ $# -gt 0 ] || { echo "sign-bin: keine Dateien" >&2; exit 2; }
if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
  echo "sign-bin: MACOS_SIGN_IDENTITY nicht gesetzt, Binaries bleiben unsigniert"
  exit 0
fi
for f in "$@"; do
  codesign_path "$f"
  codesign --verify --strict --verbose=2 "$f"
done
dir=$(mktemp -d)
zip -q -j "$dir/binaries.zip" "$@"
notarize "$dir/binaries.zip" || true
rm -rf "$dir"
