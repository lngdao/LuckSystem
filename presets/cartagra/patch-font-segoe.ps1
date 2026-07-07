param(
  [Parameter(Mandatory = $true)]
  [string]$GameDir,
  [string]$OutputRoot = (Join-Path $PSScriptRoot 'out\font_patch_segoe'),
  [string]$MarkFont = 'C:\Windows\Fonts\segoeui.ttf',
  [switch]$Install
)

$ErrorActionPreference = 'Stop'
$RepoDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$Charset = Join-Path $PSScriptRoot 'viet_composite_chars.txt'
$FontDir = Join-Path $GameDir 'files\font_win32_1920'
$SysFont = Join-Path $GameDir 'files\SYSFONT.PAK'
$Font0Out = Join-Path $OutputRoot 'font0'
$SysOut = Join-Path $OutputRoot 'sysfont'

New-Item -ItemType Directory -Force -Path $Font0Out, $SysOut | Out-Null

& go run ./tools/compositevietfont -markfont $MarkFont `
  (Join-Path $FontDir 'FONT0__INFO.PAK_OG') `
  $Charset `
  $Font0Out `
  (Join-Path $FontDir 'FONT0_GOTHIC1.PAK_OG') `
  (Join-Path $FontDir 'FONT0_GOTHIC2.PAK_OG') `
  (Join-Path $FontDir 'FONT0_GOTHIC3.PAK_OG') `
  (Join-Path $FontDir 'FONT0_MINCHO.PAK_OG') `
  (Join-Path $FontDir 'FONT0_MODERN.PAK_OG')

& go run ./tools/compositevietfont -markfont $MarkFont -sysfont `
  (Join-Path $GameDir 'files\SYSFONT.PAK_OG') `
  $Charset `
  (Join-Path $SysOut 'SYSFONT.PAK')

if ($Install) {
  $stamp = Get-Date -Format 'yyyyMMdd_HHmmss'
  $backup = Join-Path $OutputRoot "backup_before_install_$stamp"
  New-Item -ItemType Directory -Force -Path $backup | Out-Null
  Copy-Item -LiteralPath $SysFont -Destination $backup -Force
  Copy-Item -LiteralPath (Join-Path $FontDir 'FONT0*.PAK') -Destination $backup -Force
  Copy-Item -LiteralPath (Join-Path $SysOut 'SYSFONT.PAK') -Destination $SysFont -Force
  Copy-Item -LiteralPath (Join-Path $Font0Out 'FONT0*.PAK') -Destination $FontDir -Force
  Write-Host "Installed Segoe font patch. Backup: $backup"
}
