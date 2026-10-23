#!/usr/bin/env bash
# CI: installs the setup program from ../out/installer silently, checks the
# result after the installation convention and uninstalls again.
#   macOS   .pkg into ~/Applications (per user, without sudo)
#   Linux   .deb with apt (system-wide), plus a smoke test of the installed
#           binary
#   Windows Inno Setup into %LOCALAPPDATA%\Programs\pdx (per user)
set -euo pipefail
cd "$(dirname "$0")/.."
inst=../out/installer
fail() { echo "test-installer: $*" >&2; exit 1; }

case "$(uname -s)" in
Darwin)
  pkg=$(ls "$inst"/*.pkg | head -1)
  installer -pkg "$pkg" -target CurrentUserHomeDirectory
  app="$HOME/Applications/pdx Designer.app"
  [ -x "$app/Contents/MacOS/pdx-designer" ] || fail "App fehlt in ~/Applications"
  [ ! -e "/Applications/pdx Designer.app" ] || fail "systemweit statt pro Benutzer installiert"
  "$app/Contents/MacOS/pdx-designer" --smoke --open ../testdata/templates/musterrechnung
  rm -rf "$app"
  pkgutil --volume "$HOME" --forget com.github.tim-riep.pdx.designer >/dev/null 2>&1 || true
  ;;
Linux)
  deb=$(ls "$inst"/*.deb | head -1)
  sudo apt-get install -y "$(realpath "$deb")"
  [ -x /usr/bin/pdx-designer ] || fail "/usr/bin/pdx-designer fehlt"
  [ -f /usr/share/applications/pdx-designer.desktop ] || fail "Menüeintrag fehlt"
  [ -f /usr/share/icons/hicolor/256x256/apps/pdx-designer.png ] || fail "Icon fehlt"
  WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 xvfb-run -a /usr/bin/pdx-designer --smoke --open ../testdata/templates/musterrechnung
  sudo apt-get remove -y pdx-designer
  [ ! -e /usr/bin/pdx-designer ] || fail "nach Deinstallation noch vorhanden"
  ;;
MINGW*|MSYS*|CYGWIN*)
  setup=$(ls "$inst"/*-setup.exe | head -1)
  win() { cygpath -w "$1"; }
  powershell -NoProfile -Command "Start-Process -Wait -FilePath '$(win "$setup")' -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/CURRENTUSER'"
  dir="$(cygpath -u "$LOCALAPPDATA")/Programs/pdx"
  lnk="$(cygpath -u "$APPDATA")/Microsoft/Windows/Start Menu/Programs/pdx Designer.lnk"
  [ -f "$dir/pdx-designer.exe" ] || fail "pdx-designer.exe fehlt unter %LOCALAPPDATA%\\Programs\\pdx"
  [ -f "$lnk" ] || fail "Startmenüeintrag fehlt"
  [ -f "$dir/unins000.exe" ] || fail "Deinstallationsprogramm fehlt"
  powershell -NoProfile -Command "Start-Process -Wait -FilePath '$(win "$dir/unins000.exe")' -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART'"
  # Das Deinstallationsprogramm läuft als Kopie weiter, auf das Ende warten.
  for _ in $(seq 60); do [ -e "$dir/pdx-designer.exe" ] || break; sleep 1; done
  [ ! -e "$dir/pdx-designer.exe" ] && [ ! -e "$lnk" ] || fail "nach Deinstallation noch vorhanden"
  ;;
*) fail "unbekanntes System $(uname -s)" ;;
esac
echo "test-installer: ok"
