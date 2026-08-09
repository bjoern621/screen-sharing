<#
  dev-stop.ps1 - stop the backends a previous `task dev` left running, and refuse to
  hand the control pipe to a new one while something still holds it.

  Why this exists:
    A named pipe lives only as long as the process holding it, so there is no stale
    file to clear the way internal/control does on Unix - what is left behind on
    Windows is a whole backend still running. `go run` starts the compiled binary as
    a child, and a dev run that ends by closing the terminal, by a debugger detaching,
    or by anything else that does not deliver Ctrl+C to the console group leaves that
    child alive with the pipe still open. The next run then logs

      control: not serving on \\.\pipe\screenshare-control-v1: ... Zugriff verweigert

    and carries on with no control socket, because serveControl treats a socket it
    cannot open as non-fatal. That is the right call for a packaged app and the wrong
    state to develop in: the shell connects to the *older* backend, so every change
    just made appears to have done nothing. Hence the non-zero exit at the end - `task
    dev` should stop rather than start a backend no shell will ever reach.

  What it stops:
    Every backend running out of a directory this repo starts one from - the one `go
    run` links its binary into under TEMP, or build/bin from `task build:windows` -
    and not only the one holding the pipe. A leftover that lost the race for the pipe
    is just as much in the way: it is still capturing, still publishing to the relay,
    and nothing in the app's own state will ever mention it. The postcondition wanted
    here is "no backend from this tree is running", not "the pipe happens to be free".

    The build cache under LOCALAPPDATA\go-build is not one of them and is not swept:
    it holds the compiled packages and never a process. What is swept is where the
    binary is run from.

    The temp directory has no per-project path, so a second project whose main package
    is also called backend would be caught by this too. That is the price of `go run`
    naming its output after the package alone; the alternative - stopping only the
    pipe's holder - leaves the rest running.

  Idempotent: with nothing running and the pipe free it does nothing and succeeds,
  which is what lets `task dev` run it unconditionally.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

# Kept in step with internal/control/listen_windows.go, which is where the name is
# defined. The version is part of it, so a v2 backend is a different pipe and this
# script neither waits for nor reports on that one.
$pipe = "screenshare-control-v1"
$pipePath = "\\.\pipe\$pipe"

# pipeTaken answers whether the name a new backend is about to create already exists.
#
# The pipe directory is enumerated rather than probed with a client connect, because a
# connect cannot tell the two answers apart: .NET's NamedPipeClientStream polls a name
# that does not exist until the timeout runs out, so a free pipe and a pipe whose only
# instance is wedged both come back as a timeout. The directory listing is the same
# thing CreateNamedPipe collides with, so it is the question actually being asked.
# Test-Path is not it either - it reports false for a pipe that is plainly there.
#
# An enumeration that fails is reported as "not taken": this is a guard in front of a
# start that carries its own error, and a script that cannot read the pipe directory
# should not be the reason a dev run refuses to happen.
function pipeTaken {
  try {
    return [System.IO.Directory]::GetFiles("\\.\pipe\") -icontains $pipePath
  } catch {
    Write-Host "Cannot read the pipe directory ($($_.Exception.GetType().Name)); starting anyway." -ForegroundColor Yellow
    return $false
  }
}

# The temp directory is where the process this script is looking for lives. `go run`
# does not run the binary out of the build cache: it links it into a fresh
# %TEMP%\go-buildNNNNNNNN\b001\exe\backend.exe and runs that, and the cache under
# LOCALAPPDATA holds only the compiled packages behind it. A sweep that looked at the
# cache matched nothing a dev run ever started, and `task dev` reported the pipe as held
# by something it could not name - which is the state this script exists to prevent. So
# the cache is not listed: a prefix nothing can run from is a claim of coverage that
# sends the next reader looking in the wrong place.
$devTrees = @(
  (Join-Path $env:TEMP "go-build"),
  (Join-Path (Split-Path $PSScriptRoot -Parent) "build\bin")
)

$found = @(Get-CimInstance Win32_Process -Filter "Name = 'backend.exe' OR Name = 'screenshare-backend.exe'" |
  Where-Object {
    $path = $_.ExecutablePath
    $path -and @($devTrees | Where-Object { $path.StartsWith($_, [StringComparison]::OrdinalIgnoreCase) }).Count -gt 0
  })

foreach ($p in $found) {
  Write-Host "Stopping backend from an earlier dev run (pid $($p.ProcessId), started $($p.CreationDate))." -ForegroundColor Yellow
  # A process that exited between the query and here is the outcome this wants, not a
  # failure, and the same goes for the wait.
  try { Stop-Process -Id $p.ProcessId -Force -ErrorAction Stop } catch { }
  # The `go run` parent is not stopped with it: it waits on this child and exits on
  # its own once the child is gone.
  try { Wait-Process -Id $p.ProcessId -Timeout 5 -ErrorAction Stop } catch { }
}

# A process the kernel still lists after that is one that has already terminated and
# not yet been torn down: a thread of it is still waiting inside a driver, and until
# that returns the process object stays, holding every handle it had. Terminating it
# again is not possible - there is nothing left to terminate - so this only names the
# case, for the message at the end.
#
# Asking the kernel again is what tells it apart, rather than asking Get-Process
# before the kill: the two process views disagree about a process in this state, and
# which way they disagree differs between Windows PowerShell and PowerShell 7. What
# survives a Stop-Process and a wait does not.
$zombies = @()
foreach ($p in $found) {
  if (Get-CimInstance Win32_Process -Filter "ProcessId = $($p.ProcessId)") {
    $zombies += $p
  }
}

# Stopping them is not the postcondition; a free pipe is. A process can be gone with
# its pipe object still being torn down, and a backend started into that instant fails
# exactly the way the one before it did.
foreach ($attempt in 1..10) {
  if (-not (pipeTaken)) { exit 0 }
  Start-Sleep -Milliseconds 200
}

Write-Host ""
Write-Host "$pipePath is still held - the backend about to start would run with no control socket." -ForegroundColor Red
if ($zombies.Count -gt 0) {
  foreach ($z in $zombies) {
    Write-Host "  pid $($z.ProcessId), started $($z.CreationDate), terminated but not torn down - it cannot be killed and it still owns the pipe." -ForegroundColor Red
  }
  Write-Host "  A reboot is what clears that." -ForegroundColor Red
} else {
  Write-Host "  No backend from this build tree is running, so the name is being kept alive by a handle rather than by a server:" -ForegroundColor Red
  Write-Host "  a shell still connected to the backend that died keeps the pipe's last instance from being destroyed." -ForegroundColor Red
  Write-Host "  Close the app windows left over from the previous run and try again." -ForegroundColor Red
}
exit 1
