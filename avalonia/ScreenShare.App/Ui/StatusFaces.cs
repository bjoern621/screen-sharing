using Avalonia.Media;
using ScreenShare.App.Contracts;
using ScreenShare.App.Relay;

namespace ScreenShare.App.Ui;

/// <summary>
/// The four connection states every surface speaks (docs/design-language.md,
/// "Status language").
/// </summary>
public enum StatusKind
{
    Idle,
    Connecting,
    Live,
    Failed,
}

/// <summary>How one status renders: its label, its dot colour, and whether the dot pulses.</summary>
public sealed record StatusFace(string Label, Color Color, bool Pulses, bool Spins);

/// <summary>
/// The table the status vocabulary lives in. One table of facts, read by every consumer,
/// rather than the same switch restated per view (docs/development-principles.md).
/// </summary>
public static class StatusFaces
{
    /// <summary>The accent. Marks selected, active and connected - never failure.</summary>
    public static readonly Color Primary = Color.Parse("#34D399");

    /// <summary>The one danger colour. Marks failure and stop actions, never state that is merely on.</summary>
    public static readonly Color Destructive = Color.Parse("#F87171");

    /// <summary>Secondary text and the idle dot.</summary>
    public static readonly Color Muted = Color.Parse("#8B8B93");

    private static readonly Dictionary<StatusKind, StatusFace> ByKind = new()
    {
        // Idle: a small static dot. Connecting: a spinner, no dot. Live: the same small
        // dot, coloured and pulsing. Failed: the danger colour and a message beside it.
        [StatusKind.Idle] = new StatusFace("Not checked yet", Muted, Pulses: false, Spins: false),
        [StatusKind.Connecting] = new StatusFace("Checking the relay", Muted, Pulses: false, Spins: true),
        [StatusKind.Live] = new StatusFace("Connected", Primary, Pulses: true, Spins: false),
        [StatusKind.Failed] = new StatusFace("No answer from the relay", Destructive, Pulses: false, Spins: false),
    };

    public static StatusFace Of(StatusKind kind)
        => ByKind.TryGetValue(kind, out var face)
            ? face
            : Assert.Never<StatusFace>("unexpected status kind", (int)kind);

    /// <summary>
    /// Which face a poller's state wears.
    ///
    /// A completed poll outranks a running one: connecting is what the app shows before
    /// the first verdict, never what it falls back to while refreshing. A repeating poll
    /// would otherwise blink every surface between its verdict and a spinner once per
    /// interval - the live dot on a healthy relay, and the failure banner on a dead one.
    /// The status names the connection, not the request.
    /// </summary>
    public static StatusKind KindOf(RelayStatus status, bool polling)
    {
        Assert.NotNull(status, "a status kind is derived from a snapshot");

        return status.Reach switch
        {
            RelayReach.Reachable => StatusKind.Live,
            RelayReach.Unreachable => StatusKind.Failed,
            RelayReach.Unknown => polling ? StatusKind.Connecting : StatusKind.Idle,
            _ => Assert.Never<StatusKind>("unexpected relay reach", (int)status.Reach),
        };
    }
}
