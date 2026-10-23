#!/bin/sh
# Builds the image of the DSS validation instance from the pinned source.
# Downloads source code from GitHub, base images from Docker Hub and
# libraries from Maven Central. The image stays local.
set -eu
cd "$(dirname "$0")"
VERSION=$(sed -n 's/^\tVersion = "\(.*\)"/\1/p' dssval.go)
COMMIT=$(sed -n 's/^\tSourceCommit = "\(.*\)"/\1/p' dssval.go)
IMAGE=$(sed -n 's/^\tImage = "\(.*\)"/\1/p' dssval.go)
SRC=$(mktemp -d)
trap 'rm -rf "$SRC"' EXIT
git clone -q --depth 1 --branch "$VERSION" https://github.com/esig/dss-demonstrations.git "$SRC"
GOT=$(git -C "$SRC" rev-parse HEAD)
if [ "$GOT" != "$COMMIT" ]; then
  echo "Tag $VERSION zeigt auf $GOT statt $COMMIT" >&2
  exit 1
fi
docker build -t "$IMAGE" -f Dockerfile "$SRC"
