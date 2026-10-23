# Shared helpers for signing and notarizing on macOS. Sourced by the
# scripts in this folder.
#
# Environment:
#   MACOS_SIGN_IDENTITY     Developer ID Application (name or SHA-1), signs
#                           apps and binaries. Without it nothing is signed.
#   MACOS_INSTALLER_IDENTITY Developer ID Installer, signs the .pkg
#   Notarization, one of:
#   NOTARY_KEY_PATH, NOTARY_KEY_ID, NOTARY_ISSUER_ID
#                           App Store Connect API key (.p8)
#   NOTARY_PROFILE          keychain profile from
#                           xcrun notarytool store-credentials
#   Without notarization credentials signing still happens, notarization
#   is skipped with a note, unless MACOS_REQUIRE_NOTARIZATION=1.

# codesign_path signs a file or bundle with Hardened Runtime and a secure
# time stamp, both required for notarization.
codesign_path() {
  codesign --force --options runtime --timestamp --sign "$MACOS_SIGN_IDENTITY" "$1"
}

notary_args() {
  if [ -n "${NOTARY_KEY_PATH:-}" ] && [ -n "${NOTARY_KEY_ID:-}" ] && [ -n "${NOTARY_ISSUER_ID:-}" ]; then
    printf '%s\n' --key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER_ID"
  elif [ -n "${NOTARY_PROFILE:-}" ]; then
    printf '%s\n' --keychain-profile "$NOTARY_PROFILE"
  fi
}

# notarize submits a .zip, .pkg or .dmg to the notary service and waits.
# Returns 1 without credentials, unless notarization is required.
notarize() {
  args=$(notary_args)
  if [ -z "$args" ]; then
    if [ "${MACOS_REQUIRE_NOTARIZATION:-}" = 1 ]; then
      echo "notarize: keine Zugangsdaten für die Notarisierung" >&2
      exit 1
    fi
    echo "notarize: keine Zugangsdaten, $1 bleibt unnotarisiert" >&2
    return 1
  fi
  # shellcheck disable=SC2086
  out=$(xcrun notarytool submit "$1" --wait --output-format json $args)
  echo "$out"
  case "$out" in
    *'"status":"Accepted"'*) return 0 ;;
  esac
  id=$(printf '%s' "$out" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
  # shellcheck disable=SC2086
  [ -n "$id" ] && xcrun notarytool log "$id" $args >&2
  echo "notarize: $1 abgelehnt" >&2
  exit 1
}
