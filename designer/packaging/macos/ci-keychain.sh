#!/bin/sh
# CI: imports the Developer ID certificates from a base64 encoded .p12 into
# a temporary keychain and writes the identities to $GITHUB_ENV.
#
# Environment: MACOS_CERT_P12 (base64), MACOS_CERT_PASSWORD,
# optional NOTARY_KEY_P8 (content of the .p8 file), written to a file
# whose path goes to NOTARY_KEY_PATH.
set -eu
# Changes the keychain search list, therefore only in CI.
[ "${CI:-}" = true ] || { echo "ci-keychain: nur in CI, lokal signiert make mit MACOS_SIGN_IDENTITY" >&2; exit 1; }
: "${MACOS_CERT_P12:?Secret MACOS_CERT_P12 fehlt}"
: "${MACOS_CERT_PASSWORD:?Secret MACOS_CERT_PASSWORD fehlt}"
dir="${RUNNER_TEMP:-$(mktemp -d)}"
keychain="$dir/pdx-signing.keychain-db"
pass=$(openssl rand -hex 24)
printf '%s' "$MACOS_CERT_P12" | base64 --decode > "$dir/cert.p12"
security create-keychain -p "$pass" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$pass" "$keychain"
security import "$dir/cert.p12" -k "$keychain" -P "$MACOS_CERT_PASSWORD" \
  -T /usr/bin/codesign -T /usr/bin/productbuild -T /usr/bin/productsign
rm -f "$dir/cert.p12"
security set-key-partition-list -S apple-tool:,apple: -s -k "$pass" "$keychain" >/dev/null
# Intermediate certificates from Apple, otherwise Developer ID identities
# count as invalid on the runner (Developer ID G2 since 2022, plus the older one).
for ca in DeveloperIDG2CA DeveloperIDCA; do
  if curl -fsSL "https://www.apple.com/certificateauthority/$ca.cer" -o "$dir/$ca.cer"; then
    security import "$dir/$ca.cer" -k "$keychain" >/dev/null 2>&1 || true
  fi
done
security list-keychains -d user -s "$keychain" $(security list-keychains -d user | tr -d '"')
app=$(security find-identity -v -p codesigning "$keychain" | sed -n 's/.*\([0-9A-F]\{40\}\) "Developer ID Application.*/\1/p' | head -1)
inst=$(security find-identity -v "$keychain" | sed -n 's/.*\([0-9A-F]\{40\}\) "Developer ID Installer.*/\1/p' | head -1)
# Keep name, team ID and fingerprints out of all further logs
# (codesign, spctl, pkgutil and notarytool print the certificates).
if [ -n "${GITHUB_ACTIONS:-}" ]; then
  security find-identity "$keychain" | sed -n 's/.*"Developer ID [A-Za-z]*: \(.*\) (\([A-Z0-9]*\))"/\1\
\2/p' | sort -u | while IFS= read -r v; do
    [ -n "$v" ] && echo "::add-mask::$v"
  done
  for h in $(security find-identity "$keychain" | grep -o '[0-9A-F]\{40\}' | sort -u); do
    echo "::add-mask::$h"
  done
fi
if [ -z "$app" ] || [ -z "$inst" ]; then
  # Only names and validity, no keys.
  echo "ci-keychain: Zertifikate im .p12:" >&2
  security find-certificate -a "$keychain" | sed -n 's/.*"labl"<blob>="\(.*\)"/  \1/p' >&2
  echo "ci-keychain: Identitäten (auch ungültige):" >&2
  security find-identity "$keychain" | sed -E 's/[0-9A-F]{40}/<sha1>/' >&2
fi
[ -n "$app" ] || { echo "ci-keychain: kein Zertifikat Developer ID Application im .p12" >&2; exit 1; }
[ -n "$inst" ] || { echo "ci-keychain: kein Zertifikat Developer ID Installer im .p12" >&2; exit 1; }
{
  echo "MACOS_SIGN_IDENTITY=$app"
  echo "MACOS_INSTALLER_IDENTITY=$inst"
  echo "MACOS_KEYCHAIN=$keychain"
} >> "${GITHUB_ENV:-/dev/stdout}"
if [ -n "${NOTARY_KEY_P8:-}" ]; then
  printf '%s\n' "$NOTARY_KEY_P8" > "$dir/notary.p8"
  chmod 600 "$dir/notary.p8"
  echo "NOTARY_KEY_PATH=$dir/notary.p8" >> "${GITHUB_ENV:-/dev/stdout}"
fi
echo "ci-keychain: Identitäten geladen"
