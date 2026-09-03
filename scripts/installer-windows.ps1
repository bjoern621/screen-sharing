# installer-windows.ps1 - compile the Windows installer over the staged release directory.
#
#   task package:windows:installer
#
# Runs after scripts/package-windows.ps1, which is what leaves the staged directory behind:
# the installer and the zip carry the same files, so both are made from one staging step
# rather than from two assemblies free to disagree.
#
# The recipe is packaging/windows/mirrorme.iss, which states what the install does.
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
# From the run rather than from the tree, for the reason scripts/package-linux.sh states.
$version = if ($env:VERSION) { $env:VERSION.Trim() } else { 'dev' }
$dist = Join-Path $root 'build/dist'
$stage = Join-Path $dist "mirrorme-$version-windows-x86_64"
$recipe = Join-Path $root 'packaging/windows/mirrorme.iss'

# The file-version resource takes three or four fields of digits, each at most 65535,
# and a run behind no release is stamped 0.0.0.dev.<commit> (.github/workflows/version.yml).
# The leading x.y.z of that is the part the resource can carry.
# A version with no such prefix, `dev` among them, resolves to 0.0.0,
# which is the number a build nobody released deserves.
$numeric = if ($version -match '^(\d{1,5}\.\d{1,5}\.\d{1,5})') { $Matches[1] } else { '0.0.0' }

if (-not (Test-Path $stage)) {
    throw "$stage is missing: run 'task package:windows' first"
}

# Inno Setup's compiler, by the name it is on PATH under,
# and then at the two prefixes its own installer writes to.
# Chocolatey's package puts it on neither,
# so both are walked before this gives up (.github/workflows/release.yml installs it that way).
#
# Windows PowerShell 5.1 syntax throughout, which is the interpreter Taskfile.yml starts this
# under: `?.` and the other 7-only operators are parse errors there, raised before any line runs.
$command = Get-Command 'iscc.exe' -ErrorAction SilentlyContinue
$iscc = if ($command) { $command.Source } else { $null }
if (-not $iscc) {
    foreach ($base in $env:ProgramFiles, ${env:ProgramFiles(x86)}) {
        if (-not $base) { continue }
        $candidate = Join-Path $base 'Inno Setup 6/ISCC.exe'
        if (Test-Path $candidate) { $iscc = $candidate; break }
    }
}
if (-not $iscc) {
    throw "ISCC.exe not found: install Inno Setup 6 (choco install innosetup)"
}

& $iscc "/DVersion=$version" "/DNumericVersion=$numeric" "/DStage=$stage" "/DOutputDir=$dist" $recipe
if ($LASTEXITCODE -ne 0) {
    throw "iscc failed with exit code $LASTEXITCODE"
}

Write-Host "packaged $dist/mirrorme-$version-windows-x86_64-setup.exe"
