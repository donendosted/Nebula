#!/usr/bin/env sh
set -eu

if ! command -v pkgsite >/dev/null 2>&1; then
  echo "pkgsite is not installed."
  echo "Install it with:"
  echo "  go install golang.org/x/pkgsite/cmd/pkgsite@latest"
  exit 1
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

cd "$REPO_DIR"
exec pkgsite -open .
