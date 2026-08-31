using System.IO.Pipes;
using System.Net.Sockets;

namespace ScreenShare.App.Backend;

/// <summary>
/// Where the backend listens, and how a stream is opened to it.
///
/// Address is the whole discovery mechanism: no port to scan for, no file to parse, no environment variable to read,
/// and a shell that cannot open it reports the backend as not running (<c>docs/ipc-api.md</c>, "The format,
/// and why this one").
///
/// Names are the Go side's, spelled again here rather than shared,
/// and both carry the contract major (<c>backend/internal/control/listen_windows.go</c>, <c>listen_other.go</c>).
/// A <c>v2</c> is a second pipe and a second socket: two backends on different majors run side by side,
/// and a shell that opens the wrong one fails to connect instead of being turned away at <c>Hello</c>.
///
/// Unix path is placed the way Go places it, the two having to name one file.
/// Runtime directory is per user, mode 0700, cleared at logout; the fallback covers logins that have none,
/// macOS included.
/// That fallback follows Go's <c>os.UserConfigDir</c> and not .NET's nearest equivalent, which disagrees on macOS:
/// <c>SpecialFolder.ApplicationData</c> answers <c>~/.config</c> where Go answers <c>~/Library/Application Support</c>,
/// so a shell reading the wrong one reports a running backend as absent.
/// </summary>
internal static class ControlEndpoint
{
    /// <summary>
    /// Leaf alone: <see cref="NamedPipeClientStream"/> takes the server
    /// and the <c>\\.\pipe\</c> prefix as arguments of its own.
    /// </summary>
    private const string PipeName = "screenshare-control-v1";

    private const string SocketDirName = "screenshare";
    private const string SocketFileName = "control-v1.sock";

    /// <summary>
    /// Address in the form a person reads, for the sentence saying the backend is not running.
    /// Names the endpoint tried rather than the failure, the path being what makes "nothing is listening on this"
    /// actionable.
    /// </summary>
    public static string Describe() => OperatingSystem.IsWindows() ? $@"\\.\pipe\{PipeName}" : SocketPath();

    /// <summary>
    /// How long a freshly started backend is given to bind, and how often it is asked meanwhile.
    /// A started process is not a listening one, and opening the endpoint is the only signal it came up.
    /// Both short: the backend opens its socket before anything else,
    /// and a window hesitating for seconds on every launch pays for the case where there is no backend to start.
    /// </summary>
    private static readonly TimeSpan StartDeadline = TimeSpan.FromSeconds(5);
    private static readonly TimeSpan StartPoll = TimeSpan.FromMilliseconds(50);

    /// <summary>
    /// Opens one stream to the backend, which a gRPC channel calls whenever it needs a connection: the first
    /// call, and every call after one was lost.
    /// The whole of this shell's reconnect, with nothing here holding a dead handle.
    ///
    /// A refused connection throws, and throws at once on both platforms rather than waiting out a timeout.
    /// Nothing listening is the one condition the shell can act on rather than report, so it starts a backend and
    /// asks again until the deadline (<see cref="BackendProcess"/>).
    /// A start that fails, or one that never binds, leaves the original failure standing for the caller to turn
    /// into the sentence the screen shows.
    /// </summary>
    public static async ValueTask<Stream> ConnectAsync(CancellationToken cancellation)
    {
        try
        {
            return await OpenAsync(cancellation).ConfigureAwait(false);
        }
        catch (Exception) when (!cancellation.IsCancellationRequested && BackendProcess.EnsureStarted())
        {
            return await OpenStartingAsync(cancellation).ConfigureAwait(false);
        }
    }

    /// <summary>
    /// Asks the endpoint until a backend coming up answers, and rethrows the last refusal once the deadline
    /// passes.
    /// That refusal is a connect failure like the first, so the screen's sentence is the same whether a backend
    /// was started or not: nothing is listening, true either way.
    /// </summary>
    private static async ValueTask<Stream> OpenStartingAsync(CancellationToken cancellation)
    {
        var deadline = DateTime.UtcNow + StartDeadline;
        while (true)
        {
            try
            {
                return await OpenAsync(cancellation).ConfigureAwait(false);
            }
            catch (Exception) when (!cancellation.IsCancellationRequested && DateTime.UtcNow < deadline)
            {
                await Task.Delay(StartPoll, cancellation).ConfigureAwait(false);
            }
        }
    }

    /// <summary>One attempt, per platform: no retry and no start.</summary>
    private static async ValueTask<Stream> OpenAsync(CancellationToken cancellation)
    {
        if (OperatingSystem.IsWindows())
        {
            var pipe = new NamedPipeClientStream(".", PipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
            try
            {
                await pipe.ConnectAsync(cancellation).ConfigureAwait(false);
            }
            catch
            {
                // Handle belongs to this method until returned, so a throw closes it here.
                // Left open, a pipe client nobody can reach and nobody can close.
                await pipe.DisposeAsync().ConfigureAwait(false);
                throw;
            }

            return pipe;
        }

        var socket = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        try
        {
            await socket.ConnectAsync(new UnixDomainSocketEndPoint(SocketPath()), cancellation).ConfigureAwait(false);
        }
        catch
        {
            socket.Dispose();
            throw;
        }

        return new NetworkStream(socket, ownsSocket: true);
    }

    private static string SocketPath()
    {
        var dir = Environment.GetEnvironmentVariable("XDG_RUNTIME_DIR");
        if (string.IsNullOrEmpty(dir))
        {
            dir = ConfigDir();
        }

        return Path.Combine(dir, SocketDirName, SocketFileName);
    }

    /// <summary>What Go's <c>os.UserConfigDir</c> answers, per platform, so both sides name one file.</summary>
    private static string ConfigDir()
    {
        var home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);

        if (OperatingSystem.IsMacOS())
        {
            return Path.Combine(home, "Library", "Application Support");
        }

        var configured = Environment.GetEnvironmentVariable("XDG_CONFIG_HOME");
        return string.IsNullOrEmpty(configured) ? Path.Combine(home, ".config") : configured;
    }
}
