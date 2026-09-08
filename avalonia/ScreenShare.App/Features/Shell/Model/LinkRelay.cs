using System.IO;
using System.IO.Pipes;
using System.Net.Sockets;
using System.Text;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The window already open, reached by every later launch.
///
/// One window per user, and one tray icon with it.
/// A second shell draws a second grid and a second menu over the backend the first one is already using,
/// and whichever of the two started that backend takes it down for both on its quit
/// (<c>Backend/BackendProcess.cs</c>).
/// A launch offers itself here first and exits where a window takes it,
/// leaving the reader with the window they already had, which is the tray's open reached from outside.
///
/// A link rides the same hand-over.
/// The desktop starts a process per link it hands over,
/// so a link arrives as the launch carrying it,
/// and the window follows it in place of drawing a second one
/// (<c>packaging/linux/mirrorme.desktop</c>, <c>packaging/windows/mirrorme.iss</c>).
///
/// Shell to shell rather than over the control contract: what a link opens is a tile in a window, and the grid
/// is the shell's alone (<c>docs/ipc-api.md</c>, "The rule").
/// The endpoint sits beside the backend's, named per user and per instance the same way, so a build under
/// development and an installed one hand their links to their own window (<c>Backend/ControlEndpoint.cs</c>).
///
/// A launch that reaches nothing draws its own window, which is every case this cannot serve: no window open,
/// an endpoint left behind by one that crashed, a window that did not answer in time.
/// </summary>
internal static class LinkRelay
{
    /// <summary>
    /// Leaf alone: <see cref="NamedPipeClientStream"/> takes the server
    /// and the <c>\\.\pipe\</c> prefix as arguments of its own.
    /// </summary>
    private const string PipeStem = "mirrorme-link-v1";

    private const string SocketFileStem = "link-v1";
    private const string SocketFileExtension = ".sock";

    /// <summary>
    /// <c>0</c> leaves this shell out of the hand-over: it offers no launch of its own and takes no endpoint,
    /// so a window already open keeps both.
    ///
    /// For a checkout run beside an installed app,
    /// the two sharing this endpoint the way they share the backend's (<c>Backend/ControlEndpoint.cs</c>).
    /// Off, a checkout run started second exits into the installed window without drawing,
    /// and one started first swallows the launches the installed app was to answer.
    /// </summary>
    internal const string EnvOneWindow = "MIRRORME_ONE_WINDOW";

    /// <summary>
    /// How long a launch waits on a window.
    /// Short: what the wait buys is one process exiting rather than drawing,
    /// and a machine with no window open pays it before every link it opens.
    /// </summary>
    private static readonly TimeSpan Deadline = TimeSpan.FromMilliseconds(500);

    /// <summary>What a window answers once the launch is its own. Anything else is a launch that draws.</summary>
    private const string Taken = "taken";

    /// <summary>
    /// What a launch carrying no link sends, a link being the only other thing written on this wire.
    /// A line rather than nothing: a connection that writes nothing is a probe for a window
    /// and gets no answer (<see cref="Answers"/>).
    /// </summary>
    private const string Raise = "raise";

    /// <summary>
    /// Offers this launch to the window already open, and answers whether it took it.
    /// An empty <paramref name="link"/> is a launch that carried none, asking for the window alone.
    /// Blocking, and bounded by <see cref="Deadline"/>: what waits on it is the launch, before a window exists.
    /// </summary>
    public static bool TryHandOver(string link)
    {
        Assert.NotNull(link, "a hand-over names what its launch carried");

        if (!OneWindow() || !Exists())
        {
            return false;
        }

        try
        {
            using var deadline = new CancellationTokenSource(Deadline);
            return OfferAsync(link.Length > 0 ? link : Raise, deadline.Token).GetAwaiter().GetResult();
        }
        catch (Exception)
        {
            return false;
        }
    }

    /// <summary>
    /// Whether the endpoint is there at all, asked before a connection is opened.
    /// Every launch offers itself now, so the cold case is the common one, and a named pipe that is not there
    /// reads as one not created yet: given the connection alone, a first launch waits out <see cref="Deadline"/>
    /// before it draws anything (<c>Backend/ControlEndpoint.cs</c> bounds the same wait on the backend's pipe).
    /// A path that exists and answers nothing still fails at the connection, and that launch draws its own window.
    /// </summary>
    private static bool Exists()
        => File.Exists(OperatingSystem.IsWindows() ? @"\\.\pipe\" + PipeName() : SocketPath());

    /// <summary>Whether this shell is in the hand-over at all (<see cref="EnvOneWindow"/>).</summary>
    private static bool OneWindow() => Environment.GetEnvironmentVariable(EnvOneWindow) != "0";

    /// <summary>
    /// Takes the endpoint,
    /// so a later launch lands in <paramref name="take"/> rather than in a window of its own.
    /// <paramref name="take"/> gets the link that launch carried, empty where it carried none,
    /// and runs off the UI thread.
    /// Idempotent, and silent where another window holds the endpoint: the launches are that window's.
    /// </summary>
    public static void Listen(Action<string> take)
    {
        Assert.NotNull(take, "a listener names what a launch lands in");

        if (!OneWindow())
        {
            return;
        }

        if (OperatingSystem.IsWindows())
        {
            ListenOnPipe(take);
        }
        else
        {
            ListenOnSocket(take);
        }
    }

    private static async Task<bool> OfferAsync(string carried, CancellationToken cancellation)
    {
        await using var stream = await OpenAsync(cancellation).ConfigureAwait(false);

        var offer = Encoding.UTF8.GetBytes(carried + "\n");
        await stream.WriteAsync(offer, cancellation).ConfigureAwait(false);
        await stream.FlushAsync(cancellation).ConfigureAwait(false);

        using var reader = new StreamReader(stream, Encoding.UTF8, leaveOpen: true);
        var answer = await reader.ReadLineAsync(cancellation).ConfigureAwait(false);
        return answer == Taken;
    }

    private static async Task<Stream> OpenAsync(CancellationToken cancellation)
    {
        if (OperatingSystem.IsWindows())
        {
            var pipe = new NamedPipeClientStream(".", PipeName(), PipeDirection.InOut, PipeOptions.Asynchronous);
            await pipe.ConnectAsync((int)Deadline.TotalMilliseconds, cancellation).ConfigureAwait(false);
            return pipe;
        }

        var socket = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        try
        {
            await socket.ConnectAsync(new UnixDomainSocketEndPoint(SocketPath()), cancellation).ConfigureAwait(false);
        }
        catch (Exception)
        {
            socket.Dispose();
            throw;
        }

        return new NetworkStream(socket, ownsSocket: true);
    }

    /// <summary>
    /// A pipe of one instance, so the second window's creation fails and leaves the launches to the first.
    /// </summary>
    private static void ListenOnPipe(Action<string> take)
    {
        NamedPipeServerStream server;
        try
        {
            server = NextPipe();
        }
        catch (IOException)
        {
            return;
        }

        _ = ServePipeAsync(server, take);
    }

    private static NamedPipeServerStream NextPipe() => new(
        PipeName(), PipeDirection.InOut, maxNumberOfServerInstances: 1,
        PipeTransmissionMode.Byte, PipeOptions.Asynchronous);

    private static async Task ServePipeAsync(NamedPipeServerStream server, Action<string> take)
    {
        while (true)
        {
            try
            {
                await using (server)
                {
                    await server.WaitForConnectionAsync().ConfigureAwait(false);
                    await TakeAsync(server, take).ConfigureAwait(false);
                }

                server = NextPipe();
            }
            catch (Exception)
            {
                // The endpoint is gone, and a later launch draws its own window, which is where this started.
                return;
            }
        }
    }

    /// <summary>
    /// A socket at a path, which outlives the window that bound it,
    /// so a path nothing answers is removed before this one binds.
    /// </summary>
    private static void ListenOnSocket(Action<string> take)
    {
        var path = SocketPath();
        if (Answers(path))
        {
            return;
        }

        var listener = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(path)!);
            File.Delete(path);
            listener.Bind(new UnixDomainSocketEndPoint(path));
            listener.Listen(backlog: 4);
        }
        catch (Exception)
        {
            listener.Dispose();
            return;
        }

        _ = ServeSocketAsync(listener, path, take);
    }

    /// <summary>Whether something is listening at that path, which is a window holding the endpoint.</summary>
    private static bool Answers(string path)
    {
        if (!File.Exists(path))
        {
            return false;
        }

        using var probe = new Socket(AddressFamily.Unix, SocketType.Stream, ProtocolType.Unspecified);
        try
        {
            probe.Connect(new UnixDomainSocketEndPoint(path));
            return true;
        }
        catch (SocketException)
        {
            return false;
        }
    }

    private static async Task ServeSocketAsync(Socket listener, string path, Action<string> take)
    {
        using (listener)
        {
            while (true)
            {
                try
                {
                    var connection = await listener.AcceptAsync().ConfigureAwait(false);
                    await using var stream = new NetworkStream(connection, ownsSocket: true);
                    await TakeAsync(stream, take).ConfigureAwait(false);
                }
                catch (Exception)
                {
                    break;
                }
            }
        }

        try
        {
            File.Delete(path);
        }
        catch (IOException)
        {
        }
    }

    /// <summary>
    /// Reads what one launch carried off a connection, hands it over, and says so.
    /// The answer goes after the hand-over, it being what the launch waits for before it exits.
    /// </summary>
    private static async Task TakeAsync(Stream stream, Action<string> take)
    {
        using var reader = new StreamReader(stream, Encoding.UTF8, leaveOpen: true);
        var carried = await reader.ReadLineAsync().ConfigureAwait(false);
        if (string.IsNullOrEmpty(carried))
        {
            return;
        }

        take(carried == Raise ? "" : carried);

        await stream.WriteAsync(Encoding.UTF8.GetBytes(Taken + "\n")).ConfigureAwait(false);
        await stream.FlushAsync().ConfigureAwait(false);
    }

    private static string PipeName() => PipeStem + ControlEndpoint.InstanceSuffix();

    private static string SocketPath() => Path.Combine(
        ControlEndpoint.RuntimeDir(), SocketFileStem + ControlEndpoint.InstanceSuffix() + SocketFileExtension);
}
