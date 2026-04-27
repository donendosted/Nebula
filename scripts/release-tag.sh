#!/usr/bin/env sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/release-tag.sh vX.Y.Z" >&2
  exit 1
fi

VERSION="$1"

gh auth status >/dev/null
gh repo view >/dev/null

git add .
git commit -m "release" || true
git tag "$VERSION"
git push origin HEAD
git push origin "$VERSION"

echo "Pushed tag $VERSION"
