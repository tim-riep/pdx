#!/bin/sh
# Signs pdx Designer.app with Developer ID and Hardened Runtime, notarizes
# it and staples the ticket, so Gatekeeper accepts it offline as well.
# Without MACOS_SIGN_IDENTITY the script does nothing.
#
#   packaging/macos/sign-app.sh "../out/pdx Designer.app"
set -eu
. "$(dirname "$0")/lib.sh"
app=${1:?App fehlt}
if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
  echo "sign-app: MACOS_SIGN_IDENTITY nicht gesetzt, App bleibt unsigniert"
  exit 0
fi
# Innen nach außen: erst das Binary, dann das Bündel.
codesign_path "$app/Contents/MacOS/pdx-designer"
codesign_path "$app"
codesign --verify --strict --verbose=2 "$app"
zip=$(mktemp -d)/pdx-designer.zip
ditto -c -k --keepParent "$app" "$zip"
if notarize "$zip"; then
  xcrun stapler staple "$app"
  spctl --assess --type execute --verbose=2 "$app"
fi
rm -f "$zip"
