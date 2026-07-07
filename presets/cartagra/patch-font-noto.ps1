param(
  [Parameter(Mandatory = $true)]
  [string]$GameDir,
  [string]$OutputRoot = (Join-Path $PSScriptRoot 'out\font_patch_noto'),
  [string]$MarkFont = 'C:\Windows\Fonts\NotoSans-Regular.ttf',
  [switch]$Install
)

$ErrorActionPreference = 'Stop'
& (Join-Path $PSScriptRoot 'patch-font-segoe.ps1') `
  -GameDir $GameDir `
  -OutputRoot $OutputRoot `
  -MarkFont $MarkFont `
  -Install:$Install
