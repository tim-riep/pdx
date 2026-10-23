#!/bin/sh
# Builds the macOS installer package from ../out/pdx Designer.app (make
# bundle). The package installs per user into ~/Applications without
# administrator rights (installer domain currentUserHome).
#
#   packaging/build-pkg.sh <output.pkg>
set -eu
cd "$(dirname "$0")/.."
out=${1:?Ausgabedatei fehlt}
version=$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/app/app.go)
app="../out/pdx Designer.app"
[ -d "$app" ] || { echo "pdx Designer.app fehlt, erst make bundle" >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/root" "$work/resources"
# Ohne erweiterte Attribute, sonst landen ._-Dateien im Paket.
export COPYFILE_DISABLE=1
ditto --norsrc --noextattr --noqtn "$app" "$work/root/pdx Designer.app"
pkgbuild --analyze --root "$work/root" "$work/component.plist" >/dev/null
# An Ort und Stelle installieren, nicht eine vorhandene Kopie woanders
# ersetzen.
plutil -replace 0.BundleIsRelocatable -bool NO "$work/component.plist"
pkgbuild --quiet --root "$work/root" --component-plist "$work/component.plist" \
  --identifier com.github.tim-riep.pdx.designer --version "$version" \
  --install-location /Applications "$work/component.pkg"
# Nutzlast und Stückliste selbst schreiben: pkgbuild packt erweiterte
# Attribute wie com.apple.provenance als ._-Dateien ein, cpio im Format odc
# kennt keine. Dazu verlangt pkgbuild immer Administratorrechte
# (auth="root"), für die Installation pro Benutzer reicht auth="none".
pkgutil --expand "$work/component.pkg" "$work/expanded"
rm -f "$work/expanded/Payload" "$work/expanded/Bom"
(cd "$work/root" && find . | LC_ALL=C sort | cpio -o --quiet --format odc -R 0:80 | gzip -9 -n) > "$work/expanded/Payload"
mkbom "$work/root" "$work/expanded/Bom"
files=$(cd "$work/root" && find . | wc -l | tr -d ' ')
kbytes=$(du -sk "$work/root" | cut -f1)
sed -i '' -e 's/auth="root"/auth="none"/' \
  -e "s/numberOfFiles=\"[0-9]*\"/numberOfFiles=\"$files\"/" \
  -e "s/installKBytes=\"[0-9]*\"/installKBytes=\"$kbytes\"/" "$work/expanded/PackageInfo"
pkgutil --flatten "$work/expanded" "$work/pdx-designer.pkg"

cp ../LICENSE "$work/resources/LICENSE.txt"
sed "s/@VERSION@/$version/g" packaging/macos/welcome.html > "$work/resources/welcome.html"
sed "s/@VERSION@/$version/g" packaging/macos/distribution.xml > "$work/distribution.xml"
rm -f "$out"
sign=""
if [ -n "${MACOS_INSTALLER_IDENTITY:-}" ]; then
  sign="--sign $MACOS_INSTALLER_IDENTITY --timestamp"
fi
# shellcheck disable=SC2086
productbuild --quiet --distribution "$work/distribution.xml" --package-path "$work" \
  --resources "$work/resources" $sign "$out"
if [ -n "$sign" ]; then
  . packaging/macos/lib.sh
  pkgutil --check-signature "$out"
  if notarize "$out"; then
    xcrun stapler staple "$out"
    spctl --assess --type install --verbose=2 "$out"
  fi
fi
echo "$out"
