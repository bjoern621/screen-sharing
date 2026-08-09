using ScreenShare.App.Backend;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// Which decode a tile draws from: one the relay is being read for, or the local preview of
/// the stream this machine is publishing.
///
/// <b>It is the contract's own distinction and not a second one.</b> <c>FrameSubscribe</c>
/// names either a <c>WatchKey</c> or the running publish's preview, because the two are
/// different kinds of thing: a relay decode is identified by the stream and the protocol it
/// crossed the relay over, and the preview is identified by nothing at all - there is at most
/// one publish, and the preview is part of it. This type is that oneof, so the two screens
/// that put a tile on the air pick between them once here rather than each in their own way.
///
/// <b>What it is not is a leg.</b> The preview's frames never reached the relay: the publish
/// child copies its already-encoded video to a loopback port and the backend decodes it there
/// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws"). Giving it a
/// transport name would put a protocol in the table every consumer reads, and nothing could
/// be done with it.
/// </summary>
public sealed record TileSource
{
    private TileSource(string name, string transport, bool preview)
    {
        Name = name;
        Transport = transport;
        IsPreview = preview;
    }

    /// <summary>A decode of one stream on one leg, which the backend opened with <c>StartReceive</c>.</summary>
    public static TileSource Relay(string streamName, string transport)
    {
        Contracts.Assert.That(streamName.Length > 0, "a relay tile names the stream it draws");
        Contracts.Assert.That(transport.Length > 0, "a relay tile names the leg its stream is decoded from", streamName);

        return new TileSource(streamName, transport, preview: false);
    }

    /// <summary>
    /// The local preview of the running publish. The stream name is carried for what a heading
    /// prints and for nothing else: the subscription names no stream, because the backend has
    /// only one publish to preview.
    /// </summary>
    public static TileSource Preview(string streamName)
    {
        Contracts.Assert.That(streamName.Length > 0, "a preview tile names the stream it is previewing");

        return new TileSource(streamName, "", preview: true);
    }

    /// <summary>The stream name, carried and never parsed.</summary>
    public string Name { get; }

    /// <summary>The leg the decode was opened on, and empty for the preview, which crossed none.</summary>
    public string Transport { get; }

    /// <summary>Whether this tile draws the publish's local preview rather than a relay decode.</summary>
    public bool IsPreview { get; }

    /// <summary>
    /// Subscribes to this source's frames. It opens no decode either way - the relay's is
    /// <c>StartReceive</c>'s and the preview's is the publish's - so a caller that has not
    /// established one is refused rather than served.
    /// </summary>
    public Task<FrameChannel> OpenAsync(IBackend backend, CancellationToken cancellation)
    {
        Contracts.Assert.NotNull(backend, "a tile subscribes to the backend's frames");

        return IsPreview
            ? backend.OpenPreviewFramesAsync(cancellation)
            : backend.OpenFramesAsync(Name, Transport, cancellation);
    }
}
