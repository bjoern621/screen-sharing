using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// Which of the three pictures a tile draws from.
///
/// <b>It is the contract's own distinction and not a second one.</b> <c>FrameSubscribe</c> names a
/// <c>WatchKey</c>, the running publish's preview, or one of this machine's monitors, because the three are
/// different kinds of thing: a relay decode is identified by the stream and the protocol it crossed the relay
/// over, the publish preview is identified by nothing at all - there is at most one publish, and the preview
/// is part of it - and a monitor preview by the index its output is enumerated under.
/// This type is that oneof, so the screens that put a tile on the air pick between them once here rather than
/// each in their own way.
///
/// <b>What two of them are not is a leg.</b> The publish preview's frames never reached the relay: the
/// publish child copies its already-encoded video to a loopback port and the backend decodes it there
/// (<c>docs/viewer-architecture.md</c>).
/// A monitor preview's were never encoded or carried at all - the capture element hands raw pictures to the
/// render chain.
/// Giving either a transport name would put a protocol in the table every consumer reads, and nothing could
/// be done with it.
/// </summary>
public sealed record TileSource
{
    private TileSource(TileSourceKind kind, string name, string transport, int monitor)
    {
        Kind = kind;
        Name = name;
        Transport = transport;
        Monitor = monitor;
    }

    /// <summary>A decode of one stream on one leg, which the backend opened with <c>StartReceive</c>.</summary>
    public static TileSource Relay(string streamName, string transport)
    {
        Contracts.Assert.That(streamName.Length > 0, "a relay tile names the stream it draws");
        Contracts.Assert.That(transport.Length > 0, "a relay tile names the leg its stream is decoded from", streamName);

        return new TileSource(TileSourceKind.Relay, streamName, transport, monitor: 0);
    }

    /// <summary>
    /// The local preview of the running publish.
    /// The stream name is carried for what a heading prints and for nothing else: the subscription names no
    /// stream, because the backend has only one publish to preview.
    /// </summary>
    public static TileSource Preview(string streamName)
    {
        Contracts.Assert.That(streamName.Length > 0, "a preview tile names the stream it is previewing");

        return new TileSource(TileSourceKind.PublishPreview, streamName, "", monitor: 0);
    }

    /// <summary>
    /// One of this machine's screens, read live.
    /// The heading is written by the caller, because what a screen is called is a size, a refresh rate and
    /// whether it is the main one - all of which the catalog carries and this type does not.
    /// </summary>
    public static TileSource MonitorPreview(int monitor, string heading)
    {
        Contracts.Assert.That(monitor >= 0, "a screen tile names an enumerated output", monitor);
        Contracts.Assert.That(heading.Length > 0, "a screen tile names the screen it draws", monitor);

        return new TileSource(TileSourceKind.MonitorPreview, heading, "", monitor);
    }

    public TileSourceKind Kind { get; }

    /// <summary>What the heading prints. Carried and never parsed.</summary>
    public string Name { get; }

    /// <summary>The leg the decode was opened on, and empty for the two that crossed none.</summary>
    public string Transport { get; }

    /// <summary>The output's index, meaningful on <see cref="TileSourceKind.MonitorPreview"/> alone.</summary>
    public int Monitor { get; }

    /// <summary>Whether this tile draws the publish's local preview rather than a relay decode.</summary>
    public bool IsPreview => Kind == TileSourceKind.PublishPreview;

    /// <summary>
    /// Whether this tile draws a relay decode, which is the one kind with a leg to name and the one kind an
    /// audio branch can belong to.
    /// </summary>
    public bool IsRelay => Kind == TileSourceKind.Relay;

    /// <summary>Why the tile is dark when nothing is producing this picture, in this source's own terms.</summary>
    public string Missing => Kind switch
    {
        TileSourceKind.MonitorPreview => "Nothing is reading this screen.",
        _ => "Nothing is decoding this stream.",
    };

    /// <summary>
    /// Subscribes to this source's frames.
    /// It opens nothing either way - the relay's decode is <c>StartReceive</c>'s, the publish preview's is
    /// the publish's, and a screen's is <c>StartMonitorPreview</c>'s - so a caller that has not established
    /// one is refused rather than served.
    /// </summary>
    public Task<FrameChannel> OpenAsync(IBackend backend, CancellationToken cancellation)
    {
        Contracts.Assert.NotNull(backend, "a tile subscribes to the backend's frames");

        return Kind switch
        {
            TileSourceKind.PublishPreview => backend.OpenPreviewFramesAsync(cancellation),
            TileSourceKind.MonitorPreview => backend.OpenMonitorFramesAsync(Monitor, cancellation),
            _ => backend.OpenFramesAsync(Name, Transport, cancellation),
        };
    }
}

/// <summary>
/// The three pictures the frame channel carries, as the shell picks between them.
///
/// An explicit discriminator rather than a flag, for the reason the backend's own <c>FrameSource</c> has one:
/// two kinds can be told apart by whether the one that carries a key has it, and three cannot.
/// </summary>
public enum TileSourceKind
{
    /// <summary>A decode of one stream on one leg, pulled off the relay.</summary>
    Relay,

    /// <summary>The running publish's local decode of what it is sending.</summary>
    PublishPreview,

    /// <summary>One of this machine's screens, read live before anything is published.</summary>
    MonitorPreview,
}
