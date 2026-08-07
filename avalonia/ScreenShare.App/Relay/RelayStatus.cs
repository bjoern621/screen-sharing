namespace ScreenShare.App.Relay;

/// <summary>
/// How the last attempt to reach the relay ended. Three states, not a bool: a relay
/// nobody has polled yet is not the same thing as one that answered nothing, and the
/// status vocabulary in docs/design-language.md needs to tell them apart.
/// </summary>
public enum RelayReach
{
    /// <summary>No poll has completed yet.</summary>
    Unknown,

    /// <summary>The last poll returned a path list.</summary>
    Reachable,

    /// <summary>The last poll failed. <see cref="RelayStatus.Error"/> says how.</summary>
    Unreachable,
}

/// <summary>One live stream as shown to the UI.</summary>
public sealed record RelayPath
{
    public required string Name { get; init; }

    public required bool Ready { get; init; }

    /// <summary>The track names the relay reports, joined with commas, as text for the UI.</summary>
    public required string Tracks { get; init; }

    /// <summary>
    /// The video track's bitstream format in the vocabulary the codec table keys on,
    /// empty for a path whose tracks name none. It decides which protocols can carry the
    /// stream, so every consumer reads it rather than parsing <see cref="Tracks"/> its own way.
    /// </summary>
    public required string Format { get; init; }

    public required int Readers { get; init; }

    /// <summary>Live ingest bitrate, derived from byte deltas between two polls.</summary>
    public required double InMbps { get; init; }
}

/// <summary>
/// One full relay snapshot, and the only thing <see cref="RelayClient"/> returns. An
/// unreachable relay is an environment condition rather than an error, so it is reported
/// inside the status and the UI displays it like any other state.
/// </summary>
public sealed record RelayStatus
{
    public required RelayReach Reach { get; init; }

    public string Error { get; init; } = "";

    public IReadOnlyList<RelayPath> Paths { get; init; } = [];

    public bool Reachable => Reach == RelayReach.Reachable;

    /// <summary>The state before the first poll completes.</summary>
    public static readonly RelayStatus Unknown = new() { Reach = RelayReach.Unknown };

    public static RelayStatus Failed(string error) => new() { Reach = RelayReach.Unreachable, Error = error };

    public static RelayStatus Live(IReadOnlyList<RelayPath> paths)
        => new() { Reach = RelayReach.Reachable, Paths = paths };
}
