using System.Diagnostics;

namespace ScreenShare.App.Backend;

/// <summary>
/// The backend, started by the shell when nothing is listening on the control endpoint.
/// Nothing else starts it on a packaged install: no service, no launcher, no tray.
///
/// A fallback and never the first move.
/// The shell connects first, so a backend already up (a <c>task dev</c> run, a second window) is used rather
/// than duplicated.
/// A spawn that fails changes nothing on screen: the connect failure stands, naming the endpoint
/// (<c>README.md</c>, "Running it").
///
/// One this shell started dies with it, since the backend supervises encoder children and one left behind
/// goes on publishing with nothing on screen to say so.
/// One it did not start is left running.
/// </summary>
internal static class BackendProcess
{
    /// <summary>Binary name the build tasks emit (<c>Taskfile.yml</c>, <c>build</c>).</summary>
    private const string ExeName = "screenshare-backend";

    /// <summary>Serialises the start, so connect attempts racing at startup produce one backend.</summary>
    private static readonly Lock Gate = new();

    /// <summary>
    /// The backend this shell started, null until it has started one.
    /// Kept so a second connect failure starts no second backend, and so the exit hook has something to stop.
    /// </summary>
    private static Process? _started;

    /// <summary>
    /// Starts the backend unless this shell already has one, and reports whether one is on its way up.
    /// False where no binary was found or the operating system refused to run it, neither of which is worth a
    /// sentence of its own beside the connect failure the caller already shows.
    /// </summary>
    public static bool EnsureStarted()
    {
        lock (Gate)
        {
            if (_started is { HasExited: false })
            {
                // The caller waits on the endpoint rather than on this process: the socket is what it needs.
                return true;
            }

            var exe = Locate();
            if (exe is null)
            {
                return false;
            }

            try
            {
                _started = Process.Start(new ProcessStartInfo(exe)
                {
                    // The backend writes to its own run logs and has nothing to show, and a console flashing
                    // up beside the window would be the one visible sign that this app is two programs.
                    UseShellExecute = false,
                    CreateNoWindow = true,
                    WorkingDirectory = Path.GetDirectoryName(exe) ?? "",
                });
            }
            catch (Exception)
            {
                // Umgebungsfehler whichever it was: a binary that will not execute, a policy refusing it, a
                // directory that vanished.
                _started = null;
                return false;
            }

            if (_started is not null)
            {
                AppDomain.CurrentDomain.ProcessExit += StopStarted;
            }
            return _started is not null;
        }
    }

    /// <summary>
    /// Stops the backend this shell started, on the way out, and its children with it.
    /// The backend supervises the encoder and viewer processes it spawned, so a kill that took the parent
    /// alone would leave those encoding.
    /// </summary>
    private static void StopStarted(object? sender, EventArgs e)
    {
        lock (Gate)
        {
            if (_started is null || _started.HasExited)
            {
                return;
            }

            try
            {
                _started.Kill(entireProcessTree: true);
            }
            catch (Exception)
            {
                // Exited between the check and the kill, or the kill was refused.
                // Neither holds up the shell's own exit.
            }
        }
    }

    /// <summary>
    /// The backend binary: beside this shell, then the directory above it, then on PATH.
    /// null where none of the three has it.
    ///
    /// Beside first is what makes a packaged install work unconfigured, and it is the order the backend
    /// resolves ffmpeg and the GStreamer launcher in (<c>backend/internal/ffmpeg/exe.go</c>).
    /// The directory above is searched because the build tasks put the shell in <c>build/bin/avalonia</c> and
    /// the backend in <c>build/bin</c>.
    /// </summary>
    private static string? Locate()
    {
        var name = OperatingSystem.IsWindows() ? ExeName + ".exe" : ExeName;

        var beside = AppContext.BaseDirectory;
        foreach (var dir in new[] { beside, Path.GetDirectoryName(beside.TrimEnd(Path.DirectorySeparatorChar)) })
        {
            if (string.IsNullOrEmpty(dir))
            {
                continue;
            }

            var candidate = Path.Combine(dir, name);
            if (File.Exists(candidate))
            {
                return candidate;
            }
        }

        var path = Environment.GetEnvironmentVariable("PATH");
        if (string.IsNullOrEmpty(path))
        {
            return null;
        }

        foreach (var dir in path.Split(Path.PathSeparator))
        {
            if (dir.Length == 0)
            {
                continue;
            }

            try
            {
                var candidate = Path.Combine(dir, name);
                if (File.Exists(candidate))
                {
                    return candidate;
                }
            }
            catch (ArgumentException)
            {
                // A PATH entry holding characters no path may.
                // Skipped rather than fatal: the rest of the list is still worth walking.
            }
        }

        return null;
    }
}
