#!/usr/bin/env bash
# Builds the FreeRDP macOS client (MacFreeRDP.app) that the launcher bundles.
#
# On macOS the launcher is a complete package: unlike Linux, where users
# install a FreeRDP package themselves, Mac users get the RDP client inside
# the launcher .app, so a fresh Mac needs nothing but the .dmg. This script
# produces that client as a universal (arm64 + x86_64) app with OpenSSL
# linked statically, so the bundle depends on nothing outside macOS itself.
#
# Usage: launcher/scripts/build_freerdp_macos.sh [output-dir]
#   (default output: artifacts/launcher/mac-deps; result: <output-dir>/MacFreeRDP.app)
#
# Note: FreeRDP 3.3x prints a deprecation notice for this Cocoa client in
# favour of its SDL3 client. The versions here are pinned, so nothing changes
# underneath us; if a future FreeRDP drops client/Mac, switch this script to
# the SDL3 client (sdl-freerdp, needs SDL3 + SDL3_ttf built statically too)
# and adjust rdp_darwin.go's display flags (/dynamic-resolution instead of
# /smart-sizing).
#
# Pinned, checksum-verified sources are downloaded into a work directory.
# A stamp file records the versions built; re-running for the same versions
# is a no-op, so CI can cache the output directory on this script's hash.
#
# Optional environment:
#   MACOS_ARCHES         space-separated, default "arm64 x86_64" (e.g. "arm64" for a quick local build)
#   FREERDP_WORK_DIR     work directory (default: a temporary one, deleted afterwards)
#   FREERDP_TARBALL      local copy of the FreeRDP source tarball (skips the download)
#   OPENSSL_TARBALL      local copy of the OpenSSL source tarball (skips the download)
set -euo pipefail

FREERDP_VERSION=3.32.2
FREERDP_SHA256=9ccb8357bf0188044a460de775c7ea289ca7d2a2a917eca0f0a7c4dc9c8b08ea
OPENSSL_VERSION=3.5.9
OPENSSL_SHA256=603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a
DEPLOYMENT_TARGET=13.0

launcher_dir="$(cd "$(dirname "$0")/.." && pwd)"
repo_dir="$(cd "$launcher_dir/.." && pwd)"
out_dir="${1:-$repo_dir/artifacts/launcher/mac-deps}"
mkdir -p "$out_dir"
out_dir="$(cd "$out_dir" && pwd)"
app="$out_dir/MacFreeRDP.app"
stamp="$out_dir/MacFreeRDP.stamp"
arches="${MACOS_ARCHES:-arm64 x86_64}"
stamp_value="freerdp=$FREERDP_VERSION openssl=$OPENSSL_VERSION target=$DEPLOYMENT_TARGET arches=$arches"

if [ -d "$app" ] && [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$stamp_value" ]; then
  echo "MacFreeRDP.app is up to date ($stamp_value)"
  exit 0
fi

if [ -n "${FREERDP_WORK_DIR:-}" ]; then
  work="$FREERDP_WORK_DIR"
  mkdir -p "$work"
else
  work="$(mktemp -d)"
  trap 'rm -rf "$work"' EXIT
fi
jobs="$(sysctl -n hw.ncpu)"

fetch() { # fetch <url> <sha256> <dest>
  local url="$1" sha="$2" dest="$3"
  if [ ! -f "$dest" ]; then
    echo "Downloading $url"
    curl -sSL --fail -o "$dest" "$url"
  fi
  echo "$sha  $dest" | shasum -a 256 -c - >/dev/null || { echo "checksum mismatch for $dest" >&2; exit 1; }
}

openssl_tarball="${OPENSSL_TARBALL:-$work/openssl-$OPENSSL_VERSION.tar.gz}"
freerdp_tarball="${FREERDP_TARBALL:-$work/freerdp-$FREERDP_VERSION.tar.gz}"
fetch "https://github.com/openssl/openssl/releases/download/openssl-$OPENSSL_VERSION/openssl-$OPENSSL_VERSION.tar.gz" "$OPENSSL_SHA256" "$openssl_tarball"
fetch "https://github.com/FreeRDP/FreeRDP/archive/refs/tags/$FREERDP_VERSION.tar.gz" "$FREERDP_SHA256" "$freerdp_tarball"

# Keep Homebrew / MacPorts out of the build: anything found there would be
# linked dynamically and break on a machine without it.
export PKG_CONFIG_LIBDIR=/nonexistent
export PKG_CONFIG_PATH=
ignore_prefixes="/opt/homebrew;/usr/local;/opt/local"

for arch in $arches; do
  echo "== OpenSSL $OPENSSL_VERSION ($arch)"
  ssl_src="$work/openssl-src-$arch"
  ssl_prefix="$work/openssl-$arch"
  if [ ! -f "$ssl_prefix/lib/libssl.a" ]; then
    rm -rf "$ssl_src" && mkdir -p "$ssl_src"
    tar -xzf "$openssl_tarball" -C "$ssl_src" --strip-components=1
    (
      cd "$ssl_src"
      ./Configure "darwin64-$arch-cc" no-shared no-tests no-docs no-apps \
        --prefix="$ssl_prefix" --libdir=lib "-mmacosx-version-min=$DEPLOYMENT_TARGET" >/dev/null
      make -j "$jobs" >/dev/null
      make install_sw >/dev/null
    )
  fi

  echo "== FreeRDP $FREERDP_VERSION ($arch)"
  rdp_src="$work/freerdp-src"
  if [ ! -f "$rdp_src/CMakeLists.txt" ]; then
    rm -rf "$rdp_src" && mkdir -p "$rdp_src"
    tar -xzf "$freerdp_tarball" -C "$rdp_src" --strip-components=1
  fi
  rdp_build="$work/freerdp-build-$arch"
  mkdir -p "$rdp_build"
  # Static libraries, the Cocoa client only (no X11/SDL/server), and every
  # optional codec/device feature that would pull in a third-party library
  # switched off. What remains links against macOS frameworks and OpenSSL.
  cmake -S "$rdp_src" -B "$rdp_build" -G "Unix Makefiles" \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_OSX_ARCHITECTURES="$arch" \
    -DCMAKE_OSX_DEPLOYMENT_TARGET="$DEPLOYMENT_TARGET" \
    -DCMAKE_IGNORE_PREFIX_PATH="$ignore_prefixes" \
    -DBUILD_SHARED_LIBS=OFF \
    -DOPENSSL_ROOT_DIR="$ssl_prefix" \
    -DOPENSSL_USE_STATIC_LIBS=ON \
    -DWITH_CLIENT_MAC=ON \
    -DWITH_CLIENT_SDL=OFF \
    -DWITH_X11=OFF \
    -DWITH_SERVER=OFF \
    -DWITH_PLATFORM_SERVER=OFF \
    -DWITH_SHADOW=OFF \
    -DWITH_PROXY=OFF \
    -DWITH_SAMPLE=OFF \
    -DWITH_MANPAGES=OFF \
    -DWITH_BINARY_VERSIONING=OFF \
    -DWITH_JSON_DISABLED=ON \
    -DWITH_FFMPEG=OFF \
    -DWITH_SWSCALE=OFF \
    -DWITH_CAIRO=OFF \
    -DWITH_OPENH264=OFF \
    -DWITH_WEBVIEW=OFF \
    -DWITH_KRB5=OFF \
    -DWITH_PKCS11=OFF \
    -DWITH_PCSC=OFF \
    -DWITH_SMARTCARD_EMULATE=OFF \
    -DWITH_CUPS=OFF \
    -DWITH_FUSE=OFF \
    -DWITH_OPUS=OFF \
    -DWITH_LAME=OFF \
    -DWITH_FAAC=OFF \
    -DWITH_FAAD2=OFF \
    -DWITH_SOXR=OFF \
    -DWITH_GSM=OFF \
    -DWITH_LIBSYSTEMD=OFF \
    -DCHANNEL_URBDRC=OFF \
    -DWITH_UNICODE_BUILTIN=ON \
    >"$rdp_build/configure.log" 2>&1 || { tail -n 60 "$rdp_build/configure.log" >&2; exit 1; }
  cmake --build "$rdp_build" --target MacFreeRDP -j "$jobs" >"$rdp_build/build.log" 2>&1 || { tail -n 60 "$rdp_build/build.log" >&2; exit 1; }
done

# Assemble one universal bundle from the per-arch builds. The Makefile
# generator produces a bare executable (FreeRDP only builds a bundle under
# Xcode), so the .app is put together here: the configured Info.plist, the
# nibs ibtool compiled (MainMenu is the menu bar, the other two are the
# password and certificate dialogs), strings, icon and licenses.
first_arch="${arches%% *}"
first_build="$work/freerdp-build-$first_arch"
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources/en.lproj"
binaries=()
for arch in $arches; do
  binaries+=("$work/freerdp-build-$arch/client/Mac/cli/MacFreeRDP")
done
lipo -create -output "$app/Contents/MacOS/MacFreeRDP" "${binaries[@]}"
cp "$first_build/client/Mac/cli/PkgInfo" "$app/Contents/PkgInfo"
cp -R "$first_build/client/Mac/cli/MainMenu.nib" "$app/Contents/Resources/"
cp -R "$first_build/client/Mac/PasswordDialog.nib" "$app/Contents/Resources/"
cp -R "$first_build/client/Mac/CertificateDialog.nib" "$app/Contents/Resources/"
cp "$rdp_src/client/Mac/cli/en.lproj/InfoPlist.strings" "$app/Contents/Resources/en.lproj/"
cp "$rdp_src/client/Mac/cli/en.lproj/Credits.rtf" "$app/Contents/Resources/en.lproj/"
cp "$rdp_src/LICENSE" "$app/Contents/Resources/LICENSE-FreeRDP.txt"
cp "$work/openssl-src-$first_arch/LICENSE.txt" "$app/Contents/Resources/LICENSE-OpenSSL.txt"
# The session window is what the user sees in the Dock, so it gets the
# launcher's own icon when available (FreeRDP's otherwise).
if [ -f "$launcher_dir/macos/AppIcon.icns" ]; then
  cp "$launcher_dir/macos/AppIcon.icns" "$app/Contents/Resources/AppIcon.icns"
  icon_name=AppIcon
else
  cp "$rdp_src/client/Mac/cli/FreeRDP.icns" "$app/Contents/Resources/FreeRDP.icns"
  icon_name=FreeRDP
fi

plist="$app/Contents/Info.plist"
cp "$first_build/client/Mac/cli/Info.plist" "$plist"
plutil -replace CFBundleExecutable -string MacFreeRDP "$plist"
plutil -replace CFBundleName -string "Access Workspace Remote Desktop" "$plist"
plutil -replace CFBundleDisplayName -string "Access Workspace Remote Desktop" "$plist"
plutil -replace CFBundleIdentifier -string io.github.martinkerhat.accessworkspace.launcher.macfreerdp "$plist"
plutil -replace CFBundleShortVersionString -string "$FREERDP_VERSION" "$plist"
plutil -replace CFBundleVersion -string "$FREERDP_VERSION" "$plist"
plutil -replace CFBundleIconFile -string "$icon_name" "$plist"
plutil -replace LSMinimumSystemVersion -string "$DEPLOYMENT_TARGET" "$plist"
plutil -replace NSHumanReadableCopyright -string "FreeRDP $FREERDP_VERSION, Apache License 2.0. Bundled by Access Workspace Launcher." "$plist"
plutil -replace NSHighResolutionCapable -bool true "$plist"
plutil -lint "$plist" >/dev/null

# The whole point: nothing outside macOS may be referenced.
if otool -L "$app/Contents/MacOS/MacFreeRDP" | grep -E '^\s+/(opt|usr/local)/' ; then
  echo "MacFreeRDP links against libraries outside macOS (see above)" >&2
  exit 1
fi
echo "$stamp_value" > "$stamp"
echo "Built $app ($(lipo -archs "$app/Contents/MacOS/MacFreeRDP"))"
