param(
  [string]$Version = $env:AGY_SWAP_VERSION,
  [string]$TargetDir = $env:AGY_SWAP_TARGET_DIR,
  [switch]$Insecure = ($env:AGY_SWAP_INSECURE -eq '1')
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

if ([string]::IsNullOrWhiteSpace($Version)) { $Version = '2.1.3' }
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
  Write-Host "● Installing agy-swap $tag for windows/$arch..." -ForegroundColor Cyan
  Write-Host "  ↓ Downloading release binary from GitHub..." -ForegroundColor Gray

  $iwrParams = @{ UseBasicParsing = $true }
  if ($Insecure) {
    if ($PSVersionTable.PSVersion.Major -ge 6) {
      $iwrParams['SkipCertificateCheck'] = $true
    } else {
      [System.Net.ServicePointManager]::ServerCertificateValidationCallback = {$true}
    }
  }

  Invoke-WebRequest -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset) @iwrParams
  Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') @iwrParams

  $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } | Select-Object -First 1
  if (-not $line) { throw "Checksum entry for $asset was not found" }
  $expected = ($line -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw 'Checksum verification failed; installation aborted' }

  $shortHash = $expected.Substring(0, [Math]::Min(16, $expected.Length))
  Write-Host "  ✓ Cryptographic SHA-256 integrity verified: $shortHash..." -ForegroundColor Green

  $reportedVersion = (& (Join-Path $tmp $asset) --version | Out-String).Trim()
  if (-not $reportedVersion.StartsWith("agy-swap v$Version")) {
    throw "Downloaded binary reported an unexpected version: $reportedVersion"
  }
  Write-Host "  ✓ Binary self-test passed: $reportedVersion" -ForegroundColor Green

  if (Test-Path $target) { Copy-Item $target $backup -Force }
  try {
    Move-Item (Join-Path $tmp $asset) $target -Force
  } catch {
    if (Test-Path $backup) { Copy-Item $backup $target -Force }
    throw
  }
  Write-Host "  ✓ Installed binary to $target" -ForegroundColor Green

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if ($userPath -split ';' -notcontains $TargetDir) {
    Write-Host "`nNote: $TargetDir is not in your user PATH." -ForegroundColor Yellow
    Write-Host "To add it permanently, run:" -ForegroundColor Yellow
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', `"`$userPath;$TargetDir`", 'User')" -ForegroundColor Cyan
  }

  Write-Host "`n🚀 Installation complete! Run 'agy-swap' to launch the interactive manager.`n" -ForegroundColor Green
} catch {
  Write-Host "`nError during installation: $_" -ForegroundColor Red
  Write-Host "If you are behind a corporate proxy or VPN with custom SSL inspection, try:" -ForegroundColor Yellow
  Write-Host "  irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex -Insecure" -ForegroundColor Gray
  Write-Host "  or compile via Go: go install github.com/aklkbqx/agy-swap/cmd/agy-swap@latest" -ForegroundColor Gray
  exit 1
} finally {
  if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue }
}
