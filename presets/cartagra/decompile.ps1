param(
  [Parameter(Mandatory = $true)]
  [string]$GameDir,
  [string]$OutDir = (Join-Path $PSScriptRoot 'out\preview_multilang')
)

$ErrorActionPreference = 'Stop'
$RepoDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

& go run . script decompile `
  -s (Join-Path $GameDir 'files\SCRIPT.PAK') `
  -c UTF-8 `
  -O (Join-Path $RepoDir 'data\CartagraHD.txt') `
  -p (Join-Path $RepoDir 'data\CartagraHD_v1.py') `
  -g CartagraHD `
  -o $OutDir `
  --no_subdir
