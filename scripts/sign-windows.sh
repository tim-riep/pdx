#!/usr/bin/env bash
# Sign the Windows executables of a draft GitHub release with the Certum
# "Open Source Code Signing" certificate (SimplySign cloud), then publish it.
#
#   scripts/sign-windows.sh v0.4.0
#   scripts/sign-windows.sh --check v0.4.0   # sign copies only, change nothing
#
# The release workflow creates every release as a DRAFT. This script
#   1. downloads the Windows archives of the draft (pdx, pdx-server and the
#      Designer) and signs every .exe in them with ssign, which talks to
#      Certum's SimplySign cloud over HTTPS
#      (https://github.com/Le-Syl21/ssign): you type the current 6-digit code
#      from the SimplySign app, so the TOTP seed never leaves your phone,
#   2. verifies the signatures with osslsigncode and replaces the archives in
#      the draft,
#   3. starts the workflow windows-installer.yml, which builds the setup from
#      the signed Designer (the setup embeds pdx-designer.exe) and runs the
#      installation test,
#   4. signs and verifies the new setup and replaces it in the draft,
#   5. writes SHA256SUMS anew over all files of the release, and
#   6. publishes the release after you confirm.
#
# Needs: gh (logged in), osslsigncode, zip, and ssign:
#   brew install rust osslsigncode
#   cargo install ssign --version 0.1.8 --locked
#
# Your Certum account e-mail comes from CERTUM_EMAIL, or is asked for.
set -euo pipefail

REPO=tim-riep/pdx
SSIGN_VERSION=0.1.8
URL="https://github.com/$REPO"

CHECK=0
if [ "${1:-}" = "--check" ]; then
  CHECK=1
  shift
fi
TAG=${1:-}
if ! [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 [--check] <tag>   (e.g. $0 v0.4.0)" >&2
  exit 2
fi

# cargo installs into ~/.cargo/bin, which is often not on PATH.
PATH="$PATH:$HOME/.cargo/bin"

for tool in gh osslsigncode ssign zip unzip shasum; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "missing: $tool (see the header of $0)" >&2
    exit 1
  fi
done
if ! ssign --version 2>/dev/null | grep -q "$SSIGN_VERSION"; then
  echo "warning: expected ssign $SSIGN_VERSION, found: $(ssign --version 2>/dev/null)" >&2
fi

draft=$(gh release view "$TAG" -R "$REPO" --json isDraft --jq .isDraft)
if [ "$CHECK" = 0 ] && [ "$draft" != "true" ]; then
  echo "$TAG is not a draft release; refusing to change a published release." >&2
  exit 1
fi

WORK=$(mktemp -d)
# ssign caches its ~30-minute signing token; nothing of it should outlive the run.
cleanup() {
  rm -rf "$WORK"
  rm -f "${XDG_RUNTIME_DIR:-$HOME/.cache}/ssign/session.json"
}
trap cleanup EXIT

# osslsigncode defaults to /etc/ssl/cert.pem, which on macOS lacks Certum's root
# (Certum Trusted Network CA 2); the Mozilla bundle from Homebrew has it, as
# does Windows' own root store.
CAFILE=""
for c in "$(brew --prefix 2>/dev/null)/etc/ca-certificates/cert.pem" /etc/ssl/certs/ca-certificates.crt /etc/ssl/cert.pem; do
  if [ -f "$c" ]; then
    CAFILE=$c
    break
  fi
done

if [ -z "${CERTUM_EMAIL:-}" ]; then
  read -r -p "Certum account e-mail: " CERTUM_EMAIL
fi
export CERTUM_EMAIL

# sign <description> <files…>: asks for the current code and signs. The code
# goes to ssign through the environment, not the command line, so it shows up
# neither in the shell history nor in the process list.
sign() {
  local name=$1
  shift
  read -r -s -p "Current 6-digit code from the SimplySign app: " CERTUM_TOKEN
  echo
  CERTUM_TOKEN=$CERTUM_TOKEN ssign -n "$name" -u "$URL" "$@"
  CERTUM_TOKEN=""
}

# verify <files…>: every file carries a valid signature with timestamp.
verify() {
  local f out
  for f in "$@"; do
    if ! out=$(osslsigncode verify -CAfile "$CAFILE" -TSA-CAfile "$CAFILE" -in "$f" 2>&1); then
      echo "$out" >&2
      echo "verification failed for ${f##*/}; the draft was not changed." >&2
      exit 1
    fi
    echo "   ${f##*/}: $(echo "$out" | grep -m1 'Subject:' | sed 's/^[[:space:]]*//')"
  done
}

echo "== downloading the Windows archives of $TAG"
mkdir -p "$WORK/zip" "$WORK/x"
gh release download "$TAG" -R "$REPO" -p '*windows-*.zip' -D "$WORK/zip"
shopt -s nullglob
zips=("$WORK"/zip/*.zip)
if [ ${#zips[@]} -eq 0 ]; then
  echo "no Windows archives in $TAG" >&2
  exit 1
fi
for z in "${zips[@]}"; do
  echo "   ${z##*/}"
  unzip -q "$z" -d "$WORK/x"
done
exes=("$WORK"/x/*/*.exe)
if [ ${#exes[@]} -eq 0 ]; then
  echo "no executables in the Windows archives" >&2
  exit 1
fi

echo "== signing ${#exes[@]} executables"
sign "pdx" "${exes[@]}"
echo "== verifying"
verify "${exes[@]}"

# Each archive holds one folder named like the archive.
for z in "${zips[@]}"; do
  name=$(basename "$z" .zip)
  rm -f "$z"
  (cd "$WORK/x" && zip -qrX "$z" "$name")
done

if [ "$CHECK" = 1 ]; then
  echo "check passed: signing and verification work; nothing was uploaded."
  exit 0
fi

echo "== replacing the archives in the draft"
gh release upload "$TAG" -R "$REPO" --clobber "${zips[@]}"

echo "== building the setup from the signed Designer (windows-installer.yml)"
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
gh workflow run windows-installer.yml -R "$REPO" -f tag="$TAG"
run=""
for _ in $(seq 30); do
  sleep 5
  run=$(gh run list -R "$REPO" --workflow windows-installer.yml --event workflow_dispatch \
    --created ">=$since" --limit 1 --json databaseId --jq '.[0].databaseId // empty')
  [ -n "$run" ] && break
done
if [ -z "$run" ]; then
  echo "the workflow run did not show up; the draft has signed archives but an old setup." >&2
  exit 1
fi
echo "   $URL/actions/runs/$run"
if ! gh run watch "$run" -R "$REPO" --exit-status >/dev/null; then
  echo "the setup build failed: $URL/actions/runs/$run" >&2
  exit 1
fi

setup="pdx-designer-$TAG-windows-amd64-setup.exe"
mkdir -p "$WORK/setup"
gh release download "$TAG" -R "$REPO" -p "$setup" -D "$WORK/setup"
echo "== signing the setup (a new code)"
sign "pdx Designer Setup" "$WORK/setup/$setup"
echo "== verifying"
verify "$WORK/setup/$setup"
gh release upload "$TAG" -R "$REPO" --clobber "$WORK/setup/$setup"

echo "== writing SHA256SUMS anew"
mkdir -p "$WORK/all"
gh release download "$TAG" -R "$REPO" -D "$WORK/all"
rm -f "$WORK/all/SHA256SUMS"
(cd "$WORK/all" && shasum -a 256 * > SHA256SUMS)
gh release upload "$TAG" -R "$REPO" --clobber "$WORK/all/SHA256SUMS"

read -r -p "Publish $TAG now? [y/N] " answer
if [ "$answer" = "y" ] || [ "$answer" = "Y" ]; then
  gh release edit "$TAG" -R "$REPO" --draft=false --latest
  echo "published: $URL/releases/tag/$TAG"
else
  echo "left as a draft; publish later with: gh release edit $TAG -R $REPO --draft=false --latest"
fi
