# Renders the browser-extension icon set (16/32/48/128 px PNGs) into
# browser-extension/chrome/icons and browser-extension/firefox/icons.
# Design: app-styled navy rounded square with a yellow key glyph — geometry is
# proportional so every size renders sharp instead of scaling one bitmap.
# Store listings (Chrome Web Store) additionally want a 128 px store icon;
# icon-128.png doubles as that upload.

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir

$navy = [System.Drawing.Color]::FromArgb(255, 23, 42, 66)      # app panel navy
$yellow = [System.Drawing.Color]::FromArgb(255, 242, 198, 75)  # app CTA yellow

function New-RoundedRectPath([float]$x, [float]$y, [float]$w, [float]$h, [float]$r) {
  $path = New-Object System.Drawing.Drawing2D.GraphicsPath
  $d = $r * 2
  $path.AddArc($x, $y, $d, $d, 180, 90)
  $path.AddArc($x + $w - $d, $y, $d, $d, 270, 90)
  $path.AddArc($x + $w - $d, $y + $h - $d, $d, $d, 0, 90)
  $path.AddArc($x, $y + $h - $d, $d, $d, 90, 90)
  $path.CloseFigure()
  return $path
}

function Write-Icon([int]$size, [string]$outPath) {
  $bitmap = New-Object System.Drawing.Bitmap($size, $size)
  $g = [System.Drawing.Graphics]::FromImage($bitmap)
  try {
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.Clear([System.Drawing.Color]::Transparent)
    $s = [float]$size

    $navyBrush = New-Object System.Drawing.SolidBrush($navy)
    $yellowBrush = New-Object System.Drawing.SolidBrush($yellow)

    # Background: rounded square over the full canvas.
    $bg = New-RoundedRectPath 0 0 $s $s ($s * 0.22)
    $g.FillPath($navyBrush, $bg)

    # Key bow: yellow ring (filled circle + navy hole).
    $bowCx = $s * 0.36
    $bowCy = $s * 0.5
    $bowR = $s * 0.175
    $holeR = $s * 0.085
    $g.FillEllipse($yellowBrush, $bowCx - $bowR, $bowCy - $bowR, $bowR * 2, $bowR * 2)
    $g.FillEllipse($navyBrush, $bowCx - $holeR, $bowCy - $holeR, $holeR * 2, $holeR * 2)

    # Key stem with two teeth pointing down.
    $stemH = $s * 0.115
    $stemY = $bowCy - $stemH / 2
    $stemX = $bowCx + $bowR * 0.75
    $stemEnd = $s * 0.86
    $g.FillRectangle($yellowBrush, $stemX, $stemY, $stemEnd - $stemX, $stemH)
    $toothW = $s * 0.075
    $toothH = $s * 0.13
    $g.FillRectangle($yellowBrush, $stemEnd - $toothW, $stemY + $stemH, $toothW, $toothH)
    $g.FillRectangle($yellowBrush, $stemEnd - $toothW * 2.6, $stemY + $stemH, $toothW, $toothH * 0.62)

    $navyBrush.Dispose(); $yellowBrush.Dispose(); $bg.Dispose()
  } finally {
    $g.Dispose()
  }
  $bitmap.Save($outPath, [System.Drawing.Imaging.ImageFormat]::Png)
  $bitmap.Dispose()
  Write-Host "wrote $outPath"
}

foreach ($browser in @("chrome", "firefox")) {
  $iconDir = Join-Path $repoRoot "browser-extension\$browser\icons"
  New-Item -ItemType Directory -Path $iconDir -Force | Out-Null
  foreach ($size in @(16, 32, 48, 128)) {
    Write-Icon $size (Join-Path $iconDir "icon-$size.png")
  }
}
