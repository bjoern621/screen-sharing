using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What pressing commit does.
/// One control, two effects.
/// Settings cross either way; <c>PublishState.live</c> decides which (<c>docs/ipc-api.md</c>, "The rule").
/// </summary>
public enum PublishCommit
{
    /// <summary>Nothing on air, so press starts a stream through <c>StartPublish</c>.</summary>
    Start,

    /// <summary>
    /// Stream on air, so press puts these settings onto it through <c>ApplyToStream</c>.
    /// Pipeline restarts: both engines run a child built from an argv and neither takes a value back.
    /// </summary>
    Apply,
}

/// <summary>
/// Whether commit can be pressed, which effect it is, and the one sentence saying why not.
/// Every condition is a whole state another side stated, read rather than evaluated (<c>docs/ipc-api.md</c>,
/// "The rule").
/// Live stream blocks nothing; it picks <see cref="Commit"/> instead.
/// Record, so a render pass over unchanged state compares equal
/// and <see cref="ReviewStep.ViewModel.ReviewStepViewModel.Apply"/> stays idempotent.
/// </summary>
public sealed record PublishGate
{
    public required bool CanStartSharing { get; init; }

    /// <summary>
    /// Which effect the press is, for the state this gate was read from.
    /// Stated once, so label, sentence and call cannot disagree across a pass a stream started or ended in.
    /// </summary>
    public required PublishCommit Commit { get; init; }

    /// <summary>
    /// Why not pressable.
    /// Empty where pressable, and where the settings themselves block.
    /// Preflight list beside the button carries those in the backend's words.
    /// </summary>
    public required string Blocked { get; init; }

    public bool IsBlocked => Blocked.Length > 0;

    /// <summary>
    /// Gate before anything has been read: nothing committable, nothing to say.
    /// Commit from <see cref="CommitFor"/> rather than written down twice.
    /// </summary>
    public static readonly PublishGate Unread =
        new() { CanStartSharing = false, Commit = CommitFor(null), Blocked = "" };

    /// <summary>Gate for one reading of everything the commit depends on.</summary>
    /// <param name="publishable">Form's answer about the settings. False before one arrives.</param>
    /// <param name="unreachable">Why the backend could not describe the screen. Empty while it can.</param>
    /// <param name="publish">What is publishing. Null before the running state has been read.</param>
    /// <param name="relay">Relay snapshot. Null before one has been read.</param>
    /// <param name="starting">Whether a commit this flow asked for is still in flight.</param>
    public static PublishGate Of(
        bool publishable, string unreachable, PublishState? publish, RelayStatus? relay, bool starting)
    {
        Assert.NotNull(unreachable, "the gate reads the backend's own sentence, or the empty one");

        var blocked = BlockedBy(unreachable, relay);

        var gate = new PublishGate
        {
            CanStartSharing = publishable && !starting && blocked.Length == 0,
            Commit = CommitFor(publish),
            Blocked = blocked,
        };

        // Asserted where produced, not where relied on: review draws the label off this gate,
        // flow sends the effect it names.
        Assert.That(
            (gate.Commit == PublishCommit.Apply) == (publish?.Live is not null),
            "a commit that applies has a stream to apply to", gate.Commit);

        return gate;
    }

    /// <summary>
    /// Which effect the commit is, for one reading of what is publishing.
    /// One place, two readers: render draws label and sentence off the gate, press re-reads before sending,
    /// since a stream may have started or ended in between and the backend refuses each effect in the other's state.
    /// Null is a running state nothing has read, not a live stream.
    /// </summary>
    public static PublishCommit CommitFor(PublishState? publish)
        => publish?.Live is not null ? PublishCommit.Apply : PublishCommit.Start;

    /// <summary>
    /// First condition in the way, in the order a reader fixes them: unreachable backend,
    /// then relay with nothing to send to.
    /// One sentence only, the second saying no more than the first.
    /// Live stream is not among them: <see cref="PublishCommit.Apply"/> is the effect for that state.
    /// </summary>
    private static string BlockedBy(string unreachable, RelayStatus? relay)
    {
        if (unreachable.Length > 0)
        {
            return unreachable;
        }

        // Null is not unreachable: nothing has asked the relay yet.
        if (relay is null)
        {
            return "Reading what the relay is carrying.";
        }

        if (!relay.Reachable)
        {
            // Relay's own words where it gave any.
            // Zero snapshot the backend opens on carries none: a relay nothing reached, not one that refused.
            return relay.Error.Length > 0
                ? relay.Error
                : "The relay could not be reached, so there is nothing to publish to.";
        }

        return "";
    }
}
