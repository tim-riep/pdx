#!/bin/sh
# Builds the pdx-server image reproducibly for linux/amd64 and
# linux/arm64 as an OCI archive. With --check, builds twice without cache
# and compares the digests.
#
#   deploy/build.sh [--check] [target.tar]
set -eu
cd "$(dirname "$0")/.."
CHECK=0
if [ "${1:-}" = "--check" ]; then CHECK=1; shift; fi
OUT=${1:-out/pdx-server-image.tar}
mkdir -p "$(dirname "$OUT")"
SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
export SOURCE_DATE_EPOCH
build() {
  docker buildx build --no-cache --quiet \
    --platform linux/amd64,linux/arm64 \
    --build-arg SOURCE_DATE_EPOCH \
    --provenance=false --sbom=false \
    --output "type=oci,dest=$1,rewrite-timestamp=true" \
    --metadata-file "$1.json" \
    -f deploy/Dockerfile . >/dev/null
  sed -n 's/.*"containerimage.digest": *"\([^"]*\)".*/\1/p' "$1.json"
}
D1=$(build "$OUT")
echo "Digest: $D1"
if [ $CHECK -eq 1 ]; then
  D2=$(build "$OUT.2")
  rm -f "$OUT.2" "$OUT.2.json"
  if [ "$D1" != "$D2" ]; then
    echo "nicht reproduzierbar: $D1 und $D2" >&2
    exit 1
  fi
  echo "reproduzierbar"
fi
