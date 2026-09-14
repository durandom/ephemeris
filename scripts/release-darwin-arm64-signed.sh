#!/usr/bin/env bash
# Build and locally sign a darwin/arm64 ephemeris tarball.
#
# This deliberately does not notarize. A raw executable cannot be stapled, so
# a stapled/offline Gatekeeper path requires later .pkg or .app packaging.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version>" >&2
  exit 64
fi
: "${APPLE_SIGNING_IDENTITY:?set APPLE_SIGNING_IDENTITY to a Developer ID Application identity}"

version="$1"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

for command in go codesign spctl shasum tar; do
  command -v "$command" >/dev/null || {
    echo "required command not found: $command" >&2
    exit 69
  }
done

commit="$(git rev-parse HEAD)"
date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
ldflags="-s -w -X main.version=${version} -X main.commit=${commit} -X main.date=${date}"
name="ephemeris_${version}_darwin_arm64"
out="dist/signed/${name}"

rm -rf "$out"
mkdir -p "$out"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "$ldflags" -o "$out/ephemeris" ./cmd/ephemeris

codesign --force --sign "$APPLE_SIGNING_IDENTITY" --options runtime --timestamp "$out/ephemeris"
codesign --verify --strict --verbose=2 "$out/ephemeris"
spctl --assess --type execute --verbose=4 "$out/ephemeris"

cp README.md LICENSE "$out/"
tar -C dist/signed -czf "dist/signed/${name}.tar.gz" "$name"
rm -rf "$out"
(
  cd dist/signed
  shasum -a 256 "${name}.tar.gz" > "${name}.checksums.txt"
)

echo "signed artifact: dist/signed/${name}.tar.gz"
echo "checksum: dist/signed/${name}.checksums.txt"
