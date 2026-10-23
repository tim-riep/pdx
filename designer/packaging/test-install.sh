#!/usr/bin/env bash
# CI: installs the package from ../out/package/pdx-designer after the
# per-user convention, checks the installed files and uninstalls again.
# macOS and Linux use a temporary HOME, Windows the profile of the runner.
set -euo pipefail
pkg="$(cd "$(dirname "$0")/../../out/package/pdx-designer" && pwd)"
fail() { echo "test-install: $*" >&2; exit 1; }

case "$(uname -s)" in
Darwin)
  home=$(mktemp -d)
  HOME="$home" "$pkg/install.sh"
  app="$home/Applications/pdx Designer.app"
  [ -x "$app/Contents/MacOS/pdx-designer" ] || fail "App fehlt in ~/Applications"
  plutil -lint "$app/Contents/Info.plist" >/dev/null || fail "Info.plist ungültig"
  [ -f "$app/Contents/Resources/AppIcon.icns" ] || fail "AppIcon.icns fehlt"
  HOME="$home" "$pkg/install.sh" --uninstall
  [ ! -e "$app" ] || fail "App nach Deinstallation noch vorhanden"
  ;;
Linux)
  home=$(mktemp -d)
  HOME="$home" "$pkg/install.sh"
  [ -x "$home/.local/bin/pdx-designer" ] || fail "Binary fehlt in ~/.local/bin"
  grep -q "^Exec=\"$home/.local/bin/pdx-designer\"" "$home/.local/share/applications/pdx-designer.desktop" || fail "Menüeintrag falsch"
  [ -f "$home/.local/share/doc/pdx-designer/THIRD_PARTY_NOTICES.txt" ] || fail "Lizenzdateien fehlen"
  [ -f "$home/.local/share/icons/hicolor/256x256/apps/pdx-designer.png" ] || fail "Icon fehlt"
  HOME="$home" "$pkg/install.sh" --uninstall
  [ ! -e "$home/.local/bin/pdx-designer" ] || fail "Binary nach Deinstallation noch vorhanden"
  ;;
MINGW*|MSYS*|CYGWIN*)
  ps1="$(cygpath -w "$pkg/install.ps1")"
  powershell -NoProfile -ExecutionPolicy Bypass -File "$ps1"
  exe="$(cygpath -u "$LOCALAPPDATA")/Programs/pdx/pdx-designer.exe"
  lnk="$(cygpath -u "$APPDATA")/Microsoft/Windows/Start Menu/Programs/pdx Designer.lnk"
  [ -f "$exe" ] || fail "pdx-designer.exe fehlt unter %LOCALAPPDATA%\\Programs\\pdx"
  [ -f "$lnk" ] || fail "Startmenüeintrag fehlt"
  powershell -NoProfile -ExecutionPolicy Bypass -File "$ps1" -Uninstall
  [ ! -e "$exe" ] && [ ! -e "$lnk" ] || fail "nach Deinstallation noch vorhanden"
  ;;
*) fail "unbekanntes System $(uname -s)" ;;
esac
echo "test-install: ok"
