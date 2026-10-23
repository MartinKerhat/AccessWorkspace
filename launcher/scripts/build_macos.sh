#!/usr/bin/env bash
# Builds the macOS launcher: universal binary -> .app -> .dmg, optionally
# signed with a Developer ID and notarized. Runs on macOS only (cgo + Cocoa,
# codesign, hdiutil); CI and local builds on a Mac use this same script.
#
# Usage: launcher/scripts/build_macos.sh [output-dir]   (default: artifacts/launcher/mac)
#
# Optional environment:
#   MACOS_SIGN_IDENTITY  codesign identity, e.g. "Developer ID Application: Name (TEAMID)".
#                        Unset: ad-hoc signature — runs on this Mac only, Gatekeeper
#                        rejects it elsewhere.
#   NOTARY_KEY_PATH, NOTARY_KEY_ID, NOTARY_ISSUER_ID
#                        App Store Connect API key (.p8 file, key id, issuer id).
#                        All set (and an identity): notarize and staple the .dmg.
set -euo pipefail

launcher_dir="$(cd "$(dirname "$0")/.." && pwd)"
repo_dir="$(cd "$launcher_dir/.." && pwd)"
out_dir="${1:-$repo_dir/artifacts/launcher/mac}"
mkdir -p "$out_dir"
out_dir="$(cd "$out_dir" && pwd)"

version="$(sed -nE 's/^[[:space:]]*Version[[:space:]]*=[[:space:]]*"([0-9]+\.[0-9]+\.[0-9]+)".*/\1/p' "$launcher_dir/internal/launcherinfo/launcherinfo.go")"
if [ -z "$version" ]; then
  echo "could not read the launcher version" >&2
  exit 1
fi

app_name="Access Workspace Launcher"
binary_name="access-workspace-launcher"
dmg_path="$out_dir/access-workspace-launcher-macos-universal-v${version}.dmg"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
app="$work/staging/$app_name.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

echo "Building launcher $version (arm64 + x86_64)"
cd "$launcher_dir"
export CGO_ENABLED=1 GOOS=darwin MACOSX_DEPLOYMENT_TARGET=13.0
GOARCH=arm64 CC="clang -arch arm64" go build -trimpath -o "$work/$binary_name-arm64" ./cmd/access-workspace-launcher
GOARCH=amd64 CC="clang -arch x86_64" go build -trimpath -o "$work/$binary_name-amd64" ./cmd/access-workspace-launcher
lipo -create -output "$app/Contents/MacOS/$binary_name" "$work/$binary_name-arm64" "$work/$binary_name-amd64"

sed "s/__VERSION__/$version/g" "$launcher_dir/macos/Info.plist" > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist" >/dev/null
if [ -f "$launcher_dir/macos/AppIcon.icns" ]; then
  cp "$launcher_dir/macos/AppIcon.icns" "$app/Contents/Resources/AppIcon.icns"
  plutil -insert CFBundleIconFile -string AppIcon "$app/Contents/Info.plist"
fi

if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
  echo "Signing with: $MACOS_SIGN_IDENTITY"
  codesign --force --options runtime --timestamp --sign "$MACOS_SIGN_IDENTITY" "$app"
else
  echo "MACOS_SIGN_IDENTITY not set: ad-hoc signature (local testing only)"
  codesign --force --sign - "$app"
fi
codesign --verify --strict --verbose=2 "$app"

ln -s /Applications "$work/staging/Applications"
rm -f "$dmg_path"
hdiutil create -volname "$app_name" -srcfolder "$work/staging" -fs HFS+ -format UDZO -ov "$dmg_path" >/dev/null
if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
  codesign --force --timestamp --sign "$MACOS_SIGN_IDENTITY" "$dmg_path"
fi

if [ -n "${MACOS_SIGN_IDENTITY:-}" ] && [ -n "${NOTARY_KEY_PATH:-}" ] && [ -n "${NOTARY_KEY_ID:-}" ] && [ -n "${NOTARY_ISSUER_ID:-}" ]; then
  echo "Notarizing (waits for Apple)"
  notary_auth=(--key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER_ID")
  result="$(xcrun notarytool submit "$dmg_path" "${notary_auth[@]}" --wait --output-format json)"
  echo "$result"
  # submit --wait exits 0 for a rejected ("Invalid") submission too.
  status="$(plutil -extract status raw - <<<"$result")"
  if [ "$status" != "Accepted" ]; then
    xcrun notarytool log "$(plutil -extract id raw - <<<"$result")" "${notary_auth[@]}" || true
    echo "Notarization failed: $status" >&2
    exit 1
  fi
  xcrun stapler staple "$dmg_path"
  spctl --assess --type open --context context:primary-signature --verbose=2 "$dmg_path"
else
  echo "Notarization skipped (needs MACOS_SIGN_IDENTITY and the NOTARY_* variables)"
fi

echo "Built $dmg_path"
