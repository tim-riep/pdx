#!/bin/sh
# Builds the Debian package of pdx Designer from ../out/package/pdx-designer
# (make package on Linux). Unlike the per-user install script, a Debian
# package installs system-wide and pulls in GTK 4 and WebKitGTK 6:
#
#   /usr/bin/pdx-designer
#   /usr/share/applications/pdx-designer.desktop
#   /usr/share/doc/pdx-designer/
#
#   packaging/build-deb.sh <output.deb>
set -eu
cd "$(dirname "$0")/.."
out=${1:?Ausgabedatei fehlt}
version=$(sed -n 's/^const Version = "\(.*\)"/\1/p' internal/app/app.go)
pkg=../out/package/pdx-designer
[ -x "$pkg/pdx-designer" ] || { echo "$pkg/pdx-designer fehlt, erst make package" >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

root="$work/pdx-designer_${version}_amd64"
install -d "$root/DEBIAN" "$root/usr/bin" "$root/usr/share/applications" "$root/usr/share/doc/pdx-designer"
install -m 0755 "$pkg/pdx-designer" "$root/usr/bin/pdx-designer"
install -m 0644 "$pkg/README.md" "$pkg/THIRD_PARTY_NOTICES.txt" "$root/usr/share/doc/pdx-designer/"
# Debian erwartet Lizenz und Hinweise in copyright.
{
  echo "pdx Designer, https://github.com/tim-riep/pdx"
  echo
  cat "$pkg/NOTICE"
  echo
  cat "$pkg/LICENSE"
} > "$root/usr/share/doc/pdx-designer/copyright"
chmod 0644 "$root/usr/share/doc/pdx-designer/copyright"
cat > "$root/usr/share/applications/pdx-designer.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=pdx Designer
Comment=Design PDF templates against an XML schema
Exec=/usr/bin/pdx-designer
Terminal=false
Categories=Office;Development;
StartupNotify=true
DESKTOP
chmod 0644 "$root/usr/share/applications/pdx-designer.desktop"
size=$(du -sk "$root/usr" | cut -f1)
cat > "$root/DEBIAN/control" <<CONTROL
Package: pdx-designer
Version: $version
Section: graphics
Priority: optional
Architecture: amd64
Depends: libc6, libgtk-4-1 (>= 4.10), libwebkitgtk-6.0-4
Installed-Size: $size
Maintainer: Tim Riep <tim-riep@users.noreply.github.com>
Homepage: https://github.com/tim-riep/pdx
Description: Designer for PDF/A and PDF/UA documents
 Lays out documents against an XML schema without writing XML. Produces
 PDF/A and PDF/UA files with the pdx engine, checked with veraPDF.
CONTROL
dpkg-deb --root-owner-group -Zxz --build "$root" "$out" >/dev/null
echo "$out"
