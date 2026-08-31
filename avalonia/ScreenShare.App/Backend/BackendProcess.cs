using System.Diagnostics;

namespace ScreenShare.App.Backend;

/// <summary>
/// Backend, started when nothing is listening on the control endpoint.
/// Nothing else starts it on a packaged install: no service, no launcher, no tray.
///
/// Connect comes first, so a backend already up (a <c>task dev</c> run, a second window) is used rather than
/// duplicated.
/// A spawn that fails changes nothing on screen: the connect failure stands, naming the endpoint
/// (<c>README.md</c>, "Running it").
///
/// One this shell started dies with it, the backend supervising encoder children and one left behind publishing
/// with nothing on screen to say so.
/// One it did not start is left running.
/// </summary>
internal static class BackendProcess
{
    /// <summary>Binary name the build tasks emit (<c>Taskfile.yml</c>, <c>build</c>).</summary>
    private const string ExeName = "screenshare-backend";

    /// <summary>Serialises the start, so connect attempts racing at startup produce one backend.</summary>
    private static readonly Lock Gate = new();

    /// <summary>
    /// Backend this shell started. Null until one is started.
    /// Kept so a second connect failure starts no second backend, and so the exit hook has something to stop.
    /// </summary>
    private static Process? _started;

    /// <summary>
    /// Starts the backend unless this shell already has one.
    /// True where one is on its way up.
    /// False where no binary was found or the operating system refused to run it, neither worth a sentence beside
    /// the connect failure the caller already shows.
    /// </summary>
    public static bool EnsureStarted()
    {
        lock (Gate)
        {
            if (_started is { HasExited: false })
            {
                // Caller waits on the endpoint, not on this process.
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
                    // Backend writes to its own run logs, so a console beside the window would be the one visible
                    // sign this app is two programs.
                    UseShellExecute = false,
                    CreateNoWindow = true,
                    WorkingDirectory = Path.GetDirectoryName(exe) ?? "",
                });
            }
            catch (Exception)
            {
                // Umgebungsfehler: a binary that will not execute, a policy refusing it, a directory that vanished.
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
    /// Stops the backend this shell started, and its children with it.
    /// The backend supervises the encoder and viewer processes it spawned, so a kill taking the parent alone
    /// leaves those encoding.
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
                // Exited between check and kill, or kill refused.
                // Neither holds up the shell's own exit.
            }
        }
    }

    /// <summary>
    /// Backend binary: beside this shell, then the directory above it, then on PATH. Null where none has it.
    /// Beside first makes a packaged install work unconfigured, and matches the order the backend resolves ffmpeg
    /// and the GStreamer launcher in (<c>backend/internal/ffmpeg/exe.go</c>).
    /// The directory above covers the build tasks putting the shell in <c>build/bin/avalonia</c> and the backend
    /// in <c>build/bin</c>.
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
                // PATH entry holding characters no path may.
                // Skipped, the rest of the list still worth walking.
            }
        }

        return null;
    }
}
