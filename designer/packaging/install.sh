#!/bin/sh
# Installs pdx Designer for the current user, without administrator rights.
#
#   macOS: ~/Applications/pdx Designer.app
#   Linux: ~/.local/bin/pdx-designer, menu entry in
#          ~/.local/share/applications/pdx-designer.desktop,
#          license files in ~/.local/share/doc/pdx-designer
#
# Settings and autosaves are kept in the user configuration directory
# (macOS ~/Library/Application Support/pdx-designer, Linux
# ~/.config/pdx-designer) and are not touched by install or uninstall.
#
#   ./install.sh              install or update
#   ./install.sh --uninstall  remove the program and the menu entry
set -eu

here=$(cd "$(dirname "$0")" && pwd)
mode=install
case "${1:-}" in
  "") ;;
  --uninstall) mode=uninstall ;;
  -h|--help) sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) echo "unknown option: $1" >&2; exit 2 ;;
esac

case "$(uname -s)" in
Darwin)
  dest="$HOME/Applications"
  app="$dest/pdx Designer.app"
  if [ "$mode" = uninstall ]; then
    rm -rf "$app"
    echo "removed $app"
    exit 0
  fi
  if [ ! -d "$here/pdx Designer.app" ]; then
    echo "pdx Designer.app not found next to install.sh" >&2
    exit 1
  fi
  mkdir -p "$dest"
  rm -rf "$app"
  cp -R "$here/pdx Designer.app" "$dest/"
  # The release is not notarized. The user starts the installation on
  # purpose, so the quarantine flag of the download is removed.
  xattr -dr com.apple.quarantine "$app" 2>/dev/null || true
  echo "installed $app"
  ;;
Linux)
  bin="$HOME/.local/bin"
  data="${XDG_DATA_HOME:-$HOME/.local/share}"
  desktop="$data/applications/pdx-designer.desktop"
  doc="$data/doc/pdx-designer"
  if [ "$mode" = uninstall ]; then
    rm -f "$bin/pdx-designer" "$desktop"
    rm -rf "$doc"
    command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$data/applications" 2>/dev/null || true
    echo "removed $bin/pdx-designer and $desktop"
    exit 0
  fi
  if [ ! -f "$here/pdx-designer" ]; then
    echo "pdx-designer not found next to install.sh" >&2
    exit 1
  fi
  mkdir -p "$bin" "$data/applications" "$doc"
  install -m 0755 "$here/pdx-designer" "$bin/pdx-designer"
  for f in LICENSE NOTICE THIRD_PARTY_NOTICES.txt README.md; do
    [ -f "$here/$f" ] && install -m 0644 "$here/$f" "$doc/$f"
  done
  cat > "$desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=pdx Designer
Comment=Design PDF templates against an XML schema
Exec="$bin/pdx-designer"
Terminal=false
Categories=Office;Development;
StartupNotify=true
DESKTOP
  command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$data/applications" 2>/dev/null || true
  echo "installed $bin/pdx-designer"
  case ":$PATH:" in
    *":$bin:"*) ;;
    *) echo "note: $bin is not on your PATH, the menu entry works without it" ;;
  esac
  ;;
*)
  echo "unsupported system $(uname -s), on Windows use install.ps1" >&2
  exit 1
  ;;
esac
