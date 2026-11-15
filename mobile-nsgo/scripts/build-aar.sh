#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
TARGET="${1:-android_arm64}"
go tool nsgo build -embed -target "$TARGET"
AAR="$(find dist -name Bowerbird.aar -print -quit)"
if [[ -z "$AAR" ]]; then
  echo "Bowerbird.aar was not produced" >&2
  exit 1
fi
mkdir -p android/libs
cp "$AAR" android/libs/Bowerbird.aar
echo "copied $AAR -> android/libs/Bowerbird.aar"
