#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
NB_PATH="$SCRIPT_DIR/nb"
NBTUI_PATH="$SCRIPT_DIR/nbtui"
TARGET_DIR="/usr/local/bin"

if [ ! -f "$NB_PATH" ] || [ ! -f "$NBTUI_PATH" ]; then
  echo "nb and nbtui must be in the same directory as install.sh" >&2
  exit 1
fi

chmod +x "$NB_PATH" "$NBTUI_PATH"
sudo mv "$NB_PATH" "$TARGET_DIR/nb"
sudo mv "$NBTUI_PATH" "$TARGET_DIR/nbtui"

echo "Nebula installed to $TARGET_DIR"
echo "Run: nb --help"
