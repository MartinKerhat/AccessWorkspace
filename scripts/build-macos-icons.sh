#!/usr/bin/env bash
# Renders the macOS icon sets from the same design as
# scripts/build-extension-icons.ps1 (navy rounded square, yellow key), at the
# 1024 px master size Apple's icon grid needs, and packs them:
#   launcher/macos/AppIcon.icns                      — launcher app bundle
#   browser-extension/safari-app/.../AppIcon.appiconset — Safari wrapper app
# Needs ImageMagick (`magick`) and the macOS tools sips + iconutil. The outputs
# are committed, so CI does not run this; re-run after changing the design.
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

size=1024
# macOS icons sit inside a margin: Apple's template keeps the artwork within
# ~82 % of the canvas so Dock icons line up with the system ones.
inset=$(python3 -c "print(int($size * 0.09))")
s=$(python3 -c "print($size - 2 * $inset)")
f() { python3 -c "print(round($inset + $s * $1, 2))"; }
r() { python3 -c "print(round($s * $1, 2))"; }

navy="rgb(23,42,66)"
yellow="rgb(242,198,75)"

bow_cx="$(f 0.36)"; bow_cy="$(f 0.5)"; bow_r="$(r 0.175)"; hole_r="$(r 0.085)"
stem_h="$(r 0.115)"
stem_y="$(python3 -c "print(round($bow_cy - $stem_h / 2, 2))")"
stem_x="$(python3 -c "print(round($bow_cx + $bow_r * 0.75, 2))")"
stem_end="$(f 0.86)"
tooth_w="$(r 0.075)"; tooth_h="$(r 0.13)"
stem_bottom="$(python3 -c "print(round($stem_y + $stem_h, 2))")"
tooth1_x="$(python3 -c "print(round($stem_end - $tooth_w, 2))")"
tooth2_x="$(python3 -c "print(round($stem_end - $tooth_w * 2.6, 2))")"
tooth2_bottom="$(python3 -c "print(round($stem_bottom + $tooth_h * 0.62, 2))")"
tooth1_bottom="$(python3 -c "print(round($stem_bottom + $tooth_h, 2))")"
corner="$(r 0.22)"
right="$(python3 -c "print($inset + $s)")"

master="$work/icon-1024.png"
magick -size "${size}x${size}" xc:none \
  -fill "$navy" -draw "roundrectangle $inset,$inset $right,$right $corner,$corner" \
  -fill "$yellow" -draw "circle $bow_cx,$bow_cy $(python3 -c "print($bow_cx + $bow_r)"),$bow_cy" \
  -fill "$navy" -draw "circle $bow_cx,$bow_cy $(python3 -c "print($bow_cx + $hole_r)"),$bow_cy" \
  -fill "$yellow" -draw "rectangle $stem_x,$stem_y $stem_end,$stem_bottom" \
  -draw "rectangle $tooth1_x,$stem_bottom $stem_end,$tooth1_bottom" \
  -draw "rectangle $tooth2_x,$stem_bottom $(python3 -c "print($tooth2_x + $tooth_w)"),$tooth2_bottom" \
  "$master"

# .icns for the launcher bundle.
iconset="$work/AppIcon.iconset"
mkdir -p "$iconset"
for px in 16 32 128 256 512; do
  sips -z "$px" "$px" "$master" --out "$iconset/icon_${px}x${px}.png" >/dev/null
  double=$((px * 2))
  sips -z "$double" "$double" "$master" --out "$iconset/icon_${px}x${px}@2x.png" >/dev/null
done
mkdir -p "$repo_dir/launcher/macos"
iconutil -c icns "$iconset" -o "$repo_dir/launcher/macos/AppIcon.icns"
echo "wrote launcher/macos/AppIcon.icns"

# App icon set for the Safari wrapper app (Xcode asset catalog).
appiconset="$(find "$repo_dir/browser-extension/safari-app" -type d -name AppIcon.appiconset 2>/dev/null | head -n 1 || true)"
if [ -n "$appiconset" ]; then
  rm -f "$appiconset"/*.png
  json='{"images":['
  first=1
  for spec in "16:1" "16:2" "32:1" "32:2" "128:1" "128:2" "256:1" "256:2" "512:1" "512:2"; do
    pt="${spec%%:*}"; scale="${spec##*:}"
    px=$((pt * scale))
    name="icon_${pt}x${pt}@${scale}x.png"
    sips -z "$px" "$px" "$master" --out "$appiconset/$name" >/dev/null
    [ "$first" = 1 ] || json+=','
    first=0
    json+="{\"filename\":\"$name\",\"idiom\":\"mac\",\"scale\":\"${scale}x\",\"size\":\"${pt}x${pt}\"}"
  done
  json+='],"info":{"author":"xcode","version":1}}'
  printf '%s\n' "$json" | python3 -m json.tool --indent 2 > "$appiconset/Contents.json"
  echo "wrote ${appiconset#"$repo_dir"/}"
fi
