using System.IO;
using System.IO.Pipes;
using System.Net.Sockets;
using System.Text;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The window already open, reached by a launch carrying a link.
///
/// The desktop starts a process per link it hands over,
/// so following one would draw a second window over the first
/// (<c>packaging/linux/mirrorme.desktop</c>, <c>packaging/windows/mirrorme.iss</c>).
/// A launch offers its link here first and exits where a window takes it, leaving the reader with the window
/// they already had.
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
    /// How long a launch waits on a window.
    /// Short: what the wait buys is one process exiting rather than drawing,
    /// and a machine with no window open pays it before every link it opens.
    /// </summary>
    private static readonly TimeSpan Deadline = TimeSpan.FromMilliseconds(500);

    /// <summary>What a window answers once the link is its own. Anything else is a launch that draws.</summary>
    private const string Taken = "taken";

    /// <summary>
    /// Offers a link to the window already open, and answers whether it took it.
    /// Blocking, and bounded by <see cref="Deadline"/>: what waits on it is the launch, before a window exists.
    /// </summary>
    public static bool TryHandOver(string link)
    {
        Assert.That(link.Length > 0, "a hand-over carries a link");

        try
        {
            using var deadline = new CancellationTokenSource(Deadline);
            return OfferAsync(link, deadline.Token).GetAwaiter().GetResult();
        }
        catch (Exception)
        {
            return false;
        }
    }

    /// <summary>
    /// Takes the endpoint,
    /// so a later launch's link lands in <paramref name="take"/> rather than in a window of its own.
    /// Idempotent, and silent where another window holds the endpoint: the links are that window's.
    /// <paramref name="take"/> runs off the UI thread.
    /// </summary>
    public static void Listen(Action<string> take)
    {
        Assert.NotNull(take, "a listener names what a link lands in");

        if (OperatingSystem.IsWindows())
        {
            ListenOnPipe(take);
        }
        else
        {
            ListenOnSocket(take);
        }
    }

    private static async Task<bool> OfferAsync(string link, CancellationToken cancellation)
    {
        await using var stream = await OpenAsync(cancellation).ConfigureAwait(false);

        var offer = Encoding.UTF8.GetBytes(link + "\n");
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
    /// A pipe of one instance, so the second window's creation fails and leaves the links to the first.
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
    /// Reads one link off a connection, hands it over, and says so.
    /// The answer goes after the hand-over, it being what the launch waits for before it exits.
    /// </summary>
    private static async Task TakeAsync(Stream stream, Action<string> take)
    {
        using var reader = new StreamReader(stream, Encoding.UTF8, leaveOpen: true);
        var link = await reader.ReadLineAsync().ConfigureAwait(false);
        if (string.IsNullOrEmpty(link))
        {
            return;
        }

        take(link);

        await stream.WriteAsync(Encoding.UTF8.GetBytes(Taken + "\n")).ConfigureAwait(false);
        await stream.FlushAsync().ConfigureAwait(false);
    }

    private static string PipeName() => PipeStem + ControlEndpoint.InstanceSuffix();

    private static string SocketPath() => Path.Combine(
        ControlEndpoint.RuntimeDir(), SocketFileStem + ControlEndpoint.InstanceSuffix() + SocketFileExtension);
}
