using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// One stream the relay carries, and what this machine is doing about it.
///
/// Each field comes off a <see cref="RelayPath"/> or the list of open viewers.
/// No resolution, frame rate, decoder or decode load: those are a decoder's figures, and the relay's snapshot
/// carries none of them.
///
/// A record, so a render pass over an unchanged snapshot compares equal and the bound list is left alone rather
/// than repainted under the pointer on every poll.
/// </summary>
public sealed record StreamRow
{
    /// <summary>Path name, and the stream name a viewer is opened for.</summary>
    public required string Name { get; init; }

    /// <summary>
    /// Name to print: the path with the prefix this machine reaches under taken off, which the backend derives,
    /// the prefix being a group key's digest.
    /// Equal to <see cref="Name"/> where there is no prefix, so a list prints this one and never chooses.
    /// </summary>
    public required string OwnName { get; init; }

    /// <summary>
    /// A publisher is connected and the path is being served.
    /// A path the relay knows about is not yet a path with a stream on it.
    /// </summary>
    public required bool IsReady { get; init; }

    /// <summary>What the path carries, in the relay's own words.</summary>
    public required string Tracks { get; init; }

    /// <summary>
    /// Bitstream format, deciding the legs a viewer may receive it over.
    /// Empty where the relay's description named nothing this app knows, and an empty one refuses no viewer, the
    /// snapshot being able to predate the stream.
    /// </summary>
    public required string Format { get; init; }

    /// <summary>Readers the relay counts on this path, this machine's own among them.</summary>
    public required int Readers { get; init; }

    /// <summary>Ingest rate in Mb/s, computed by the backend from byte deltas between its own polls.</summary>
    public required double InMbps { get; init; }

    /// <summary>
    /// Transports this machine has an external viewer open on, in the backend's order.
    /// A stream can be watched over several at once, so the stream name alone is not an identity.
    /// </summary>
    public required IReadOnlyList<string> WatchedOn { get; init; }

    public bool IsWatched => WatchedOn.Count > 0;

    public string Detail => IsReady ? $"{InMbps:0.0} Mb/s" : "starting";
}
