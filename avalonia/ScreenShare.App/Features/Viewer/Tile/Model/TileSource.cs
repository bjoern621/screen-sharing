using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// Which picture a tile draws from.
///
/// The contract's own distinction and not a second one: <c>FrameSubscribe</c> names a <c>StreamRef</c>, the running
/// publish's preview, or one of this machine's monitors, and the identities differ in kind.
/// A relay decode is the stream together with the protocol it crossed the relay over, the publish preview is
/// identified by nothing at all there being at most one publish, and a monitor by the index its output is
/// enumerated under.
/// This type is that oneof, so the screens putting a tile on the air pick between them here rather than each in
/// their own way.
///
/// What a preview is not is a leg.
/// The publish preview's frames never reached the relay, the publish child copying its already-encoded video to a
/// loopback port the backend decodes (<c>docs/viewer-architecture.md</c>).
/// A monitor preview's were never encoded or carried at all, the capture element handing raw pictures straight to
/// the render chain.
/// A transport name on either would put a protocol in the table every consumer reads with nothing behind it.
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

    /// <summary>A decode of one stream on one leg, opened by <c>StartReceive</c>.</summary>
    public static TileSource Relay(string streamName, string transport)
    {
        Contracts.Assert.That(streamName.Length > 0, "a relay tile names the stream it draws");
        Contracts.Assert.That(transport.Length > 0, "a relay tile names the leg its stream is decoded from", streamName);

        return new TileSource(TileSourceKind.Relay, streamName, transport, monitor: 0);
    }

    /// <summary>
    /// Running publish's local preview.
    /// The stream name is carried for the heading and nothing else: the subscription names no stream, the backend
    /// having one publish to preview.
    /// </summary>
    public static TileSource Preview(string streamName)
    {
        Contracts.Assert.That(streamName.Length > 0, "a preview tile names the stream it is previewing");

        return new TileSource(TileSourceKind.PublishPreview, streamName, "", monitor: 0);
    }

    /// <summary>
    /// A screen of this machine, read live.
    /// The heading is the caller's: what a screen is called is its size, its refresh rate and whether it is the
    /// main one, all of which the catalog carries and this type does not.
    /// </summary>
    public static TileSource MonitorPreview(int monitor, string heading)
    {
        Contracts.Assert.That(monitor >= 0, "a screen tile names an enumerated output", monitor);
        Contracts.Assert.That(heading.Length > 0, "a screen tile names the screen it draws", monitor);

        return new TileSource(TileSourceKind.MonitorPreview, heading, "", monitor);
    }

    public TileSourceKind Kind { get; }

    /// <summary>What the heading prints, carried and never parsed.</summary>
    public string Name { get; }

    /// <summary>Leg the decode was opened on, empty for the kinds that crossed none.</summary>
    public string Transport { get; }

    /// <summary>Output index, meaningful on <see cref="TileSourceKind.MonitorPreview"/> alone.</summary>
    public int Monitor { get; }

    /// <summary>A relay decode, the one kind with a leg to name and the one kind an audio branch belongs to.</summary>
    public bool IsRelay => Kind == TileSourceKind.Relay;

    /// <summary>Why the tile is dark, in this source's own terms.</summary>
    public string Missing => Kind switch
    {
        TileSourceKind.MonitorPreview => "Nothing is reading this screen.",
        _ => "Nothing is decoding this stream.",
    };

    /// <summary>
    /// Opens the frames of this source.
    /// Opens no picture: the relay's decode is <c>StartReceive</c>'s, the publish preview's is the publish's, and
    /// a screen's is <c>StartMonitorPreview</c>'s, so a caller that established none is refused rather than
    /// served.
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
/// Pictures the frame channel carries, as the shell picks between them.
/// A named discriminator rather than a flag, for the reason the backend's own <c>FrameSource</c> carries one: a
/// flag separates two kinds and no more.
/// </summary>
public enum TileSourceKind
{
    /// <summary>A decode of one stream on one leg, pulled off the relay.</summary>
    Relay,

    /// <summary>Running publish's local decode of what it is sending.</summary>
    PublishPreview,

    /// <summary>One of this machine's screens, read live before anything is published.</summary>
    MonitorPreview,
}
