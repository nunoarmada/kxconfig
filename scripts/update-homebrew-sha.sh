#!/usr/bin/env bash

# Script to update the SHA256 hash in the Homebrew formula
# Usage: ./scripts/update-homebrew-sha.sh <version>
# Example: ./scripts/update-homebrew-sha.sh v1.0.0

set -euo pipefail
IFS=$'\n\t'

SCRIPT_NAME="$(basename "$0")"

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME <version>

Description:
  Updates the SHA256 hash in the Homebrew formula for the specified version.

Example:
  $SCRIPT_NAME v1.0.0
EOF
}

err() {
  echo "[$SCRIPT_NAME] ERROR: $*" >&2
}

if [ $# -eq 0 ]; then
  err "Version argument required."
  usage
  exit 1
fi

VERSION="$1"
FORMULA_FILE="Formula/kxconfig.rb"

if [ ! -f "$FORMULA_FILE" ]; then
  err "Formula file not found: $FORMULA_FILE"
  exit 1
fi

# Try different tag formats
TAG_FORMATS=(
  "${VERSION}"                    # As provided (e.g., kxconfig-v1.3.0 or v1.3.0)
  "kxconfig-${VERSION}"           # Prefixed format (e.g., kxconfig-v1.3.0)
  "v${VERSION#v}"                 # Ensure v prefix (e.g., v1.3.0)
)

URL=""
SHA256=""

for TAG in "${TAG_FORMATS[@]}"; do
  TEST_URL="https://github.com/nunoarmada/kxconfig/archive/refs/tags/${TAG}.tar.gz"
  echo "Trying tag format: ${TAG}..."
  
  HTTP_CODE=$(curl -sL -o /dev/null -w "%{http_code}" "$TEST_URL" 2>/dev/null || echo "000")
  
  if [ "$HTTP_CODE" = "200" ]; then
    URL="$TEST_URL"
    echo "Found valid tag: ${TAG}"
    SHA256=$(curl -sL "$URL" | shasum -a 256 | awk '{print $1}')
    break
  fi
done

if [ -z "$SHA256" ] || [ -z "$URL" ]; then
  err "Failed to get SHA256. Make sure the version tag exists on GitHub."
  err "Tried formats: ${TAG_FORMATS[*]}"
  exit 1
fi

echo "SHA256: $SHA256"
echo ""

# Update the formula file
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS
  sed -i '' "s|url \".*\"|url \"${URL}\"|" "$FORMULA_FILE"
  sed -i '' "s|sha256 \".*\"|sha256 \"${SHA256}\"|" "$FORMULA_FILE"
else
  # Linux
  sed -i "s|url \".*\"|url \"${URL}\"|" "$FORMULA_FILE"
  sed -i "s|sha256 \".*\"|sha256 \"${SHA256}\"|" "$FORMULA_FILE"
fi

echo "✓ Updated $FORMULA_FILE:"
echo "  URL: $URL"
echo "  SHA256: $SHA256"

