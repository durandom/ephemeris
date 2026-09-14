# Releasing ephemeris

The GitHub Actions release workflow creates unsigned darwin and Linux archives
for every `v*` tag. It intentionally has no Apple credentials and must not be
changed to add them casually.

## Local signed darwin/arm64 archive

A maintainer with the existing Apple Developer setup may make a separately
signed archive on a Mac. This is local-only: it neither publishes an archive
nor uploads a certificate or a Keychain profile anywhere.

1. Confirm the available Developer ID Application identities:

   ```sh
   security find-identity -v -p codesigning
   ```

2. Set the chosen identity and run the script from this repository:

   ```sh
   export APPLE_SIGNING_IDENTITY='Developer ID Application: <name> (<team id>)'
   scripts/release-darwin-arm64-signed.sh v0.1.0
   ```

The script builds a `CGO_ENABLED=0` darwin/arm64 binary, applies the hardened
runtime and a timestamped Developer ID signature, verifies the signature, and
writes the archive plus its SHA-256 file under `dist/signed/`. It also checks
that Gatekeeper reports the expected `Unnotarized Developer ID` status: a raw
binary cannot be stapled, so that status is expected here rather than a release
failure.

The signing identity is intentionally not hard-coded: choose an identity that
is already present in the local Keychain. Do not put a certificate, private
key, Apple ID, app-specific password, notary credential, or team secret in
this repository or GitHub Actions.

## Notarization is deferred

This release path signs a raw command-line binary. Apple can notarize a raw
binary, but it cannot staple a notarization ticket to one. Consequently this
path provides no stapled, offline Gatekeeper proof. A notarized `.pkg` or a
minimal `.app` wrapper is separate packaging work and is deliberately
out-of-scope for now. Do not describe a locally signed tarball as notarized or
publish it as a replacement for the unsigned GitHub Action artifact without a
separate release decision.
