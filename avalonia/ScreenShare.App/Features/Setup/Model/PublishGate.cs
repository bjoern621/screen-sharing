using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What pressing the commit does to the world.
///
/// One control and two effects, not two controls.
/// The configured settings cross either way.
/// What differs is whether a pipeline is already carrying them, which is <c>PublishState.live</c>, a whole
/// state the backend stated and this side reads rather than tracks (<c>docs/ipc-api.md</c>, "The rule").
/// </summary>
public enum PublishCommit
{
    /// <summary>
    /// Nothing on the air, so the press starts a stream through <c>StartPublish</c>, which persists the
    /// settings and launches an encoder on them.
    /// </summary>
    Start,

    /// <summary>
    /// A stream on the air, so the press puts these settings onto it through <c>ApplyToStream</c>.
    ///
    /// <b>The pipeline restarts.</b> Both engines run a child built from an argv and neither takes a value
    /// back afterwards, so there is no live-safe change on this contract, which is what the copy beside the
    /// button says rather than leaving a reader to discover it at their viewers.
    /// </summary>
    Apply,
}

/// <summary>
/// Whether the commit can be pressed, which effect pressing it is, and the one sentence saying why it cannot.
///
/// <b>Every condition is a whole state another side stated, read rather than evaluated.</b>
/// <c>Form.publishable</c> for the settings, the presence of <c>PublishState.live</c> for a pipeline in force,
/// <c>RelayStatus.reachable</c> for somewhere to send to, and the relay's own error text for why it is not.
/// None of it is ranked, derived or re-decided here (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>A live stream blocks nothing.</b> It decides <see cref="Commit"/> instead, stated as data, so no caller
/// reads the publish state a second time and reaches its own answer.
///
/// A record, so a render pass over unchanged state produces a value that compares equal, which is what keeps
/// <see cref="ReviewStep.ViewModel.ReviewStepViewModel.Apply"/> idempotent.
/// </summary>
public sealed record PublishGate
{
    public required bool CanStartSharing { get; init; }

    /// <summary>
    /// Which effect the press is, for the running state this gate was read from.
    /// Stated once here, so the label, the sentence under it and the call the press makes cannot disagree on
    /// the pass a stream started or ended.
    /// </summary>
    public required PublishCommit Commit { get; init; }

    /// <summary>
    /// Why it is not pressable, empty where it is, and empty where the settings themselves block it.
    /// The preflight list beside the button carries those in the backend's words, and a second sentence would
    /// be this module paraphrasing a diagnostic it did not write.
    /// </summary>
    public required string Blocked { get; init; }

    public bool IsBlocked => Blocked.Length > 0;

    /// <summary>
    /// The gate before anything has been read: nothing committable, nothing to say about it.
    /// A state rather than a gap, as an unresolved form is.
    /// Its commit comes from <see cref="CommitFor"/> rather than being written down a second time.
    /// </summary>
    public static readonly PublishGate Unread =
        new() { CanStartSharing = false, Commit = CommitFor(null), Blocked = "" };

    /// <summary>
    /// The gate for one reading of everything the commit depends on.
    /// </summary>
    /// <param name="publishable">The form's answer about the settings, false before one arrives.</param>
    /// <param name="unreachable">Why the backend could not describe the screen, empty while it can.</param>
    /// <param name="publish">What is publishing, null before the running state has been read.</param>
    /// <param name="relay">The relay snapshot, null before one has been read.</param>
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

        // Stated where it is produced rather than where it is relied on: the review draws the label off this
        // gate, and the flow sends the effect it names.
        Assert.That(
            (gate.Commit == PublishCommit.Apply) == (publish?.Live is not null),
            "a commit that applies has a stream to apply to", gate.Commit);

        return gate;
    }

    /// <summary>
    /// Which effect the commit is, for one reading of what is publishing.
    ///
    /// <b>One place, because two sides read it.</b> The render pass draws the label and the sentence off the
    /// gate this composes, and the press reads what is publishing again before sending.
    /// A press trusting the last pass would act on a stream that may have started or ended since, and the
    /// backend refuses each effect in exactly the state the other one is for.
    ///
    /// Null is a running state nothing has read, which is not a live stream: no pipeline is in force until
    /// something says one is, and the commit is locked for other reasons at that point anyway.
    /// </summary>
    public static PublishCommit CommitFor(PublishState? publish)
        => publish?.Live is not null ? PublishCommit.Apply : PublishCommit.Start;

    /// <summary>
    /// The first condition standing in the way, in the order a reader acts on them: a backend that cannot be
    /// reached at all, then a relay with nothing to send to.
    /// One sentence only, since a reader fixes them in that order and the second would say no more than the
    /// first.
    ///
    /// A stream on the air is not among them: <see cref="PublishCommit.Apply"/> is the effect for that state,
    /// so refusing here would stand in front of a call that succeeds.
    /// </summary>
    private static string BlockedBy(string unreachable, RelayStatus? relay)
    {
        if (unreachable.Length > 0)
        {
            return unreachable;
        }

        // Null is not unreachable: nothing has asked the relay yet, and reading that as a failure would name a
        // condition nobody established.
        if (relay is null)
        {
            return "Reading what the relay is carrying.";
        }

        if (!relay.Reachable)
        {
            // The relay's own words where it gave any.
            // The zero snapshot the backend opens on carries none: a relay nothing reached,
            // not one that refused.
            return relay.Error.Length > 0
                ? relay.Error
                : "The relay could not be reached, so there is nothing to publish to.";
        }

        return "";
    }
}
