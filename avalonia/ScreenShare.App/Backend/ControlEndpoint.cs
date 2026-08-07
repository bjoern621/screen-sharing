using System.IO.Pipes;
using System.Net.Sockets;

namespace ScreenShare.App.Backend;

/// <summary>
/// Where the backend listens, and how a stream is opened to it.
///
/// The address is the whole discovery mechanism. There is no port to scan for, no file to
/// parse and no environment variable to read: the backend serves the control contract on one
/// local endpoint per platform, and a shell that cannot open it reports that the backend is
/// not running (<c>docs/ipc-api.md</c>, "The format, and why this one").
///
/// <b>The names are the Go side's, spelled again here rather than shared, and both halves
/// carry the contract major.</b> A <c>v2</c> is therefore a second pipe and a second socket:
/// two backends on different majors can run side by side, and a shell that opens the wrong
/// one fails to connect instead of connecting and being turned away at <c>Hello</c>. The Go
/// halves are <c>internal/control/listen_windows.go</c> and <c>listen_other.go</c>.
///
/// The Unix path is placed the way Go places it, because the two have to name one file. The
/// runtime directory is the right home - per user, mode 0700, cleared at logout - and the
/// fallback exists for the logins that have none, macOS included. That fallback follows Go's
/// <c>os.UserConfigDir</c> rather than .NET's nearest equivalent, which disagrees with it on
/// macOS: <c>SpecialFolder.ApplicationData</c> answers <c>~/.config</c> there and Go answers
/// <c>~/Library/Application Support</c>, and a shell looking in the wrong one would report a
/// running backend as absent.
/// </summary>
internal static class ControlEndpoint
{
    /// <summary>
    /// The pipe, named as <see cref="NamedPipeClientStream"/> takes it: the server and the
    /// <c>\\.\pipe\</c> prefix are that type's own arguments, so only the leaf is written here.
    /// </summary>
    private const string PipeName = "screenshare-control-v1";

    private const string SocketDirName = "screenshare";
    private const string SocketFileName = "control-v1.sock";

    /// <summary>
    /// The address in the form a person reads, for the sentence that says the backend is not
    /// running. It names the endpoint that was tried rather than the failure, because "nothing
    /// is listening on this" is the fact, and the path is what makes it actionable.
    /// </summary>
    public static string Describe() => OperatingSystem.IsWindows() ? $@"\\.\pipe\{PipeName}" : SocketPath();

    /// <summary>
    /// Opens one stream to the backend, which is what a gRPC channel calls whenever it needs a
    /// connection - the first call, and every call after one was lost. That is the whole of
    /// this shell's reconnect: a backend that was started after the window was is reached by
    /// asking again, with nothing here holding a dead handle.
    ///
    /// A refused connection throws, and it throws quickly on both platforms: an absent pipe and
    /// an unbound socket both fail at once rather than waiting. The caller turns that into the
    /// message the screen shows.
    /// </summary>
    public static async ValueTask<Stream> ConnectAsync(CancellationToken cancellation)
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
                // The handle is this method's until it is handed over, so a connect that threw
                // closes it here. Left open it would be a pipe client nobody can reach and
                // nobody can close.
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

    /// <summary>Go's <c>os.UserConfigDir</c>, per platform, so both sides name one directory.</summary>
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
