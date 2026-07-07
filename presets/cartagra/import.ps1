param(
  [Parameter(Mandatory = $true)]
  [string]$GameDir,
  [Parameter(Mandatory = $true)]
  [string]$InputDir,
  [string]$OutPak = ''
)

$ErrorActionPreference = 'Stop'
$RepoDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$SourcePak = Join-Path $GameDir 'files\SCRIPT.PAK'
if ($OutPak -eq '') {
  $OutPak = $SourcePak
}

if (Test-Path -LiteralPath $OutPak) {
  $stamp = Get-Date -Format 'yyyyMMdd_HHmmss'
  Copy-Item -LiteralPath $OutPak -Destination "$OutPak.before_import_$stamp" -Force
}

& go run . script import `
  -s $SourcePak `
  -c UTF-8 `
  -O (Join-Path $RepoDir 'data\CartagraHD.txt') `
  -p (Join-Path $RepoDir 'data\CartagraHD_v1.py') `
  -g CartagraHD `
  -i $InputDir `
  -o $OutPak `
  --no_subdir
