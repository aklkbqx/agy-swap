param(
  [string]$Version = $env:AGY_SWAP_VERSION,
  [string]$TargetDir = $env:AGY_SWAP_TARGET_DIR
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = '2.1.2' }
if ([string]::IsNullOrWhiteSpace($TargetDir)) { $TargetDir = Join-Path $env:LOCALAPPDATA 'Programs\agy-swap' }

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($arch) {
  'x64' { $arch = 'amd64' }
  'arm64' { $arch = 'arm64' }
  default { throw "Unsupported Windows architecture: $arch" }
}
$tag = if ($Version.StartsWith('v')) { $Version } else { "v$Version" }
$Version = $tag.Substring(1)
$asset = "agy-swap_${tag}_windows_${arch}.exe"
$base = "https://github.com/aklkbqx/agy-swap/releases/download/$tag"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("agy-swap-install-" + [guid]::NewGuid().ToString('N'))
$target = Join-Path $TargetDir 'agy-swap.exe'
$backup = "$target.bak"

try {
  New-Item -ItemType Directory -Force -Path $tmp, $TargetDir | Out-Null
  Write-Host "Installing agy-swap $tag for windows/$arch..."
  Invoke-WebRequest -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
  Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
  $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } | Select-Object -First 1
  if (-not $line) { throw "Checksum entry for $asset was not found" }
  $expected = ($line -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw 'Checksum verification failed; installation aborted' }
  $reportedVersion = (& (Join-Path $tmp $asset) --version | Out-String).Trim()
  if (-not $reportedVersion.StartsWith("agy-swap v$Version")) {
    throw "Downloaded binary reported an unexpected version: $reportedVersion"
  }
  if (Test-Path $target) { Copy-Item $target $backup -Force }
  try {
    Move-Item (Join-Path $tmp $asset) $target -Force
  } catch {
    if (Test-Path $backup) { Copy-Item $backup $target -Force }
    throw
  }
  Write-Host "Installed agy-swap to $target"
  Write-Host 'Add this directory to PATH if it is not already present:'
  Write-Host "  $TargetDir"
} catch {
  Write-Error $_
  exit 1
} finally {
  if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue }
}
