#Requires -Version 7.0
<#
  relay.ps1 - run the relay and the group service beside it on Windows.

  The relay verifies every connection against the key set the group service publishes,
  so a relay started on its own refuses every publisher:
  the two come up together or neither serves anything.

  Linux and macOS take both binaries from the flake's dev shell (deploy/relay.sh),
  which Windows has none of,
  so mediamtx.exe is fetched into bin/ on first run and the group service is built out of backend/.
  Anything putting a NAT between the host and the relay is worth avoiding here:
  a UDP proxy that rewrites the source port breaks SRT's handshake with "I/O error",
  where a native binary binds :8890 on the host and SRT works.

  A machine with no deployment certificate and no hook paths takes both as environment overrides,
  the file itself being the one every relay reads (deploy/mediamtx-groups.yml).

  Foreground, and Ctrl+C ends both.

  PowerShell 7 for the certificate:
  the key is written through .NET APIs Windows PowerShell's runtime does not carry.
#>
param(
  [string]$Version = "1.20.0"
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$bin  = Join-Path $root "bin"
$exe  = Join-Path $bin "mediamtx.exe"
$conf = Join-Path $root "deploy/mediamtx-groups.yml"
# Everything drawn for a development relay, none of it committed:
# the certificate the TLS listeners hold, the key the group service signs with,
# and the MoQ pair MediaMTX draws in its working directory.
$dev  = Join-Path $root "dev-relay"
$cert = Join-Path $dev "cert.pem"
$key  = Join-Path $dev "key.pem"

New-Item -ItemType Directory -Force $dev | Out-Null

if (-not (Test-Path $exe)) {
  Write-Host "mediamtx.exe not found - downloading v$Version ..." -ForegroundColor Yellow
  New-Item -ItemType Directory -Force $bin | Out-Null
  $zip = Join-Path $bin "mediamtx.zip"
  $url = "https://github.com/bluenviron/mediamtx/releases/download/v$Version/mediamtx_v${Version}_windows_amd64.zip"
  Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
  Expand-Archive -Path $zip -DestinationPath $bin -Force
  Remove-Item $zip
  # The archive carries a configuration of its own, which is not the one this repository serves.
  Remove-Item (Join-Path $bin "mediamtx.yml") -ErrorAction SilentlyContinue
}

# Drawn where absent, so a second run keeps the certificate whoever trusted it once already holds.
# localhost and the loopback addresses, those being the names a relay on this machine is reached by.
if (-not (Test-Path $cert) -or -not (Test-Path $key)) {
  Write-Host "Drawing a development certificate into dev-relay ..." -ForegroundColor Yellow
  $ecdsa = [System.Security.Cryptography.ECDsa]::Create(
    [System.Security.Cryptography.ECCurve]::CreateFromFriendlyName("nistP256"))
  $request = [System.Security.Cryptography.X509Certificates.CertificateRequest]::new(
    "CN=localhost", $ecdsa, [System.Security.Cryptography.HashAlgorithmName]::SHA256)
  $names = [System.Security.Cryptography.X509Certificates.SubjectAlternativeNameBuilder]::new()
  $names.AddDnsName("localhost")
  $names.AddIpAddress([System.Net.IPAddress]::Loopback)
  $names.AddIpAddress([System.Net.IPAddress]::IPv6Loopback)
  $request.CertificateExtensions.Add($names.Build())
  # A day back, so a clock a few minutes behind this one still accepts it.
  $drawn = $request.CreateSelfSigned(
    [System.DateTimeOffset]::UtcNow.AddDays(-1), [System.DateTimeOffset]::UtcNow.AddDays(365))

  $wrap = [System.Base64FormattingOptions]::InsertLineBreaks
  Set-Content -Path $cert -NoNewline -Value (
    "-----BEGIN CERTIFICATE-----`n" +
    [Convert]::ToBase64String($drawn.RawData, $wrap) +
    "`n-----END CERTIFICATE-----`n")
  Set-Content -Path $key -NoNewline -Value (
    "-----BEGIN PRIVATE KEY-----`n" +
    [Convert]::ToBase64String($ecdsa.ExportPkcs8PrivateKey(), $wrap) +
    "`n-----END PRIVATE KEY-----`n")
}

Push-Location (Join-Path $root "backend")
try {
  & go build -o (Join-Path $bin "groupd.exe") ./cmd/groupd
  if ($LASTEXITCODE -ne 0) { throw "building the group service failed" }
} finally {
  Pop-Location
}

# The signing key is stored, so tokens issued before a restart are still verified after one:
# the relay caches the key set it fetched,
# and a service drawing a new key on every start would refuse every connection
# until that cache turned over.
# Reports land beside the other drawn state, so a development relay takes them like a deployment.
$service = Start-Process -FilePath (Join-Path $bin "groupd.exe") -PassThru -NoNewWindow `
  -ArgumentList "-key", (Join-Path $dev "signing-key.pem"), "-reports", (Join-Path $dev "reports")

$env:MTX_RTSPSERVERCERT = $cert
$env:MTX_RTSPSERVERKEY  = $key
$env:MTX_RTMPSERVERCERT = $cert
$env:MTX_RTMPSERVERKEY  = $key
# The read hook is a shell script and nothing here runs one,
# so this relay reports no read and enforcement waits for the group service's next reconcile.
$env:MTX_PATHDEFAULTS_RUNONREAD = ""

# From the relay's own directory, because MediaMTX draws the MoQ pair beside whatever it runs in.
Push-Location $dev
try {
  & $exe $conf
} finally {
  Pop-Location
  if (-not $service.HasExited) { $service.Kill() }
}
