using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// One stream the relay is carrying, and what this machine is doing about it.
///
/// Every field is read out of a <see cref="RelayPath"/> or off the list of open viewers. There
/// is no resolution, frame rate, decoder or decode load here, and their absence is the honest
/// shape rather than an omission: those are a <i>decoder's</i> figures, this shell decodes
/// nothing, and the relay's snapshot does not carry them. The one figure about the picture is
/// the relay's own description of the tracks, in the relay's words.
///
/// A record, so a render pass over an unchanged snapshot compares equal and the bound list is
/// left alone - a relay polled every few seconds otherwise repaints the list under the pointer.
/// </summary>
public sealed record StreamRow
{
    /// <summary>The path's name, which is also the stream name a viewer is opened for.</summary>
    public required string Name { get; init; }

    /// <summary>
    /// Whether a publisher is connected and the path is being served. A path the relay knows
    /// about is not necessarily a path with a stream on it, which is the difference between a
    /// stream that is starting and one that is running.
    /// </summary>
    public required bool IsReady { get; init; }

    /// <summary>The relay's own description of what the path carries, in its words.</summary>
    public required string Tracks { get; init; }

    /// <summary>
    /// The bitstream format, which decides the legs a viewer may receive it over. Empty where
    /// the relay's description named nothing this app recognises, and an empty format refuses
    /// no viewer: the snapshot can be older than the stream.
    /// </summary>
    public required string Format { get; init; }

    /// <summary>How many readers the relay counts on this path.</summary>
    public required int Readers { get; init; }

    /// <summary>The live ingest rate, computed by the backend from byte deltas between its own polls.</summary>
    public required double InMbps { get; init; }

    /// <summary>
    /// The transports this machine currently has an external viewer open on, in the order the
    /// backend listed them. A stream can be watched over several at once, which is why the
    /// stream name alone is not an identity.
    /// </summary>
    public required IReadOnlyList<string> WatchedOn { get; init; }

    public bool IsWatched => WatchedOn.Count > 0;

    /// <summary>What the row prints after the name: the rate while it runs, and what it is doing otherwise.</summary>
    public string Detail => IsReady ? $"{InMbps:0.0} Mb/s" : "starting";
}
