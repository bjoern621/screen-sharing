using System.Diagnostics;

namespace ScreenShare.App.Backend;

/// <summary>
/// The backend, started by the shell when nothing is listening on the control endpoint.
///
/// The backend is a headless binary and the shell is the window in front of it, so a user who opens the app
/// has asked for both.
/// Nothing else starts it on a packaged install: there is no service, no launcher and no tray, and asking the
/// reader to run a second program before the first one works is asking them to know that there are two.
///
/// <b>It is a fallback and never the first move.</b> The shell connects first and starts one only when that
/// fails, so a backend already running - a <c>task dev</c> run, a second window - is used rather than
/// duplicated.
/// A spawn that fails changes nothing the reader sees: the connect failure stands and the screen says the
/// backend is not running, with the endpoint named (<c>README.md</c>, "Running it").
///
/// <b>It dies with the window that started it.</b> The backend supervises encoder children, so one left
/// behind is a machine still publishing to a relay with nothing on screen to say so.
/// A backend this shell did not start is left alone for the same reason: it is not this window's to stop.
/// </summary>
internal static class BackendProcess
{
    /// <summary>
    /// The binary, as the build tasks name it (<c>Taskfile.yml</c>, <c>build</c>).
    /// The Windows suffix is added on Windows, the way the Go side's own lookup adds it.
    /// </summary>
    private const string ExeName = "screenshare-backend";

    /// <summary>
    /// Guards the start, so several connection attempts racing at startup produce one backend rather than one
    /// each.
    /// </summary>
    private static readonly Lock Gate = new();

    /// <summary>
    /// The backend this shell started, null while it has started none.
    /// It is kept so a second failure does not start a second one, and so the exit hook has something to
    /// stop.
    /// </summary>
    private static Process? _started;

    /// <summary>
    /// Starts the backend unless this shell already has one starting, and reports whether one is now on its
    /// way up.
    ///
    /// False means the caller's connect failure is the whole story: either the binary is not where it should
    /// be, or the operating system refused to run it.
    /// Both are conditions the screen already has a sentence for, and neither is worth a second one about a
    /// spawn the reader did not ask for.
    /// </summary>
    public static bool EnsureStarted()
    {
        lock (Gate)
        {
            if (_started is { HasExited: false })
            {
                // One is already coming up.
                // The caller waits for the endpoint rather than for this, because what it needs is the socket
                // and not the process.
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
                    // No window: the backend writes to its own run logs and has nothing to show.
                    // A console flashing up beside the window would be the one visible sign that this app is
                    // two programs, which is exactly what it is not supposed to be.
                    UseShellExecute = false,
                    CreateNoWindow = true,
                    WorkingDirectory = Path.GetDirectoryName(exe) ?? "",
                });
            }
            catch (Exception)
            {
                // Every reason a start fails here is the machine's: a binary that will not execute, a policy
                // that refuses it, a directory that vanished.
                // The screen says the backend is not running, which is true whichever of them it was.
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
    /// Stops the backend this shell started, on the way out.
    ///
    /// The whole tree, because the backend is a supervisor: its own exit path stops the encoder and viewer
    /// children it spawned, and a kill that took the parent alone would leave those behind encoding.
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
                // It ended between the check and the kill, or the operating system refused.
                // Neither is worth holding up the shell's own exit for.
            }
        }
    }

    /// <summary>
    /// The backend binary: beside this shell first, then on PATH.
    /// Null where neither has it.
    ///
    /// Beside first is what makes a packaged install work with nothing configured, and it is the same order
    /// the backend itself resolves ffmpeg and the GStreamer launcher in (<c>internal/ffmpeg/exe.go</c>).
    /// The directory above is searched too, because the build tasks put the shell in
    /// <c>build/bin/avalonia</c> and the backend in <c>build/bin</c>.
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
                // A PATH entry with characters no path may hold.
                // Skipped rather than fatal: the rest of the list is still worth walking.
            }
        }

        return null;
    }
}
