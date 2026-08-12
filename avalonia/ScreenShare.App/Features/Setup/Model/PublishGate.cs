using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// What pressing the commit will do to the world.
///
/// It is one control and two effects, not two controls. The settings the reader configured
/// cross to the backend either way; what differs is whether there is already a pipeline
/// carrying them, and that is <c>PublishState.live</c> - a whole state the backend stated,
/// read rather than tracked here (<c>docs/ipc-api.md</c>, "The rule").
/// </summary>
public enum PublishCommit
{
    /// <summary>
    /// Nothing is on the air, so the commit starts a stream: <c>StartPublish</c>, which
    /// persists the settings and launches an encoder on them.
    /// </summary>
    Start,

    /// <summary>
    /// A stream is on the air, so the commit puts these settings onto it:
    /// <c>ApplyToStream</c>.
    ///
    /// <b>It restarts the pipeline rather than changing one that keeps running.</b> Both
    /// engines run a child built from an argv and neither takes a value back afterwards, so
    /// there is no live-safe change on this contract and never was - which is why the copy
    /// beside the button says so instead of leaving a reader to discover it at their viewers.
    /// </summary>
    Apply,
}

/// <summary>
/// Whether the commit can be pressed, what pressing it will do, and the one sentence saying
/// why it cannot.
///
/// <b>Every condition here is a whole state some other side stated, read rather than
/// evaluated.</b> Whether the settings themselves publish is <c>Form.publishable</c>, which is
/// the backend's own answer and the same one that would refuse the call; whether a stream is
/// already in force is the presence of <c>PublishState.live</c>; whether the relay answered is
/// <c>RelayStatus.reachable</c>, and the reason it did not is the relay's own error text. None
/// of it is ranked, derived or re-decided here (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>A live stream is not one of the blockers, and that is the shape of this type rather than
/// a condition that was dropped.</b> It used to refuse the commit and point at the broadcast
/// screen, because the only effect the shell could reach was <c>StartPublish</c> and the
/// backend refuses that while a pipeline is in force. What a live stream decides now is
/// <see cref="Commit"/> - which of the two effects the press is - so the gate states it as data
/// rather than leaving each caller to look at the publish state again and reach its own answer.
///
/// It is a record so that a render pass over unchanged state produces a value that compares
/// equal, which is what keeps <see cref="ReviewStep.ViewModel.ReviewStepViewModel.Apply"/>
/// idempotent.
/// </summary>
public sealed record PublishGate
{
    /// <summary>Whether the one red button is pressable.</summary>
    public required bool CanStartSharing { get; init; }

    /// <summary>
    /// Which effect pressing it is, for the running state this gate was read from.
    ///
    /// It is stated here rather than derived again by whoever draws the button, so the label,
    /// the sentence under it and the call the press makes are one answer. A caller re-deriving
    /// it from the same state would be a second definition of the same fact, and two
    /// definitions disagree on exactly the pass where the stream started or ended.
    /// </summary>
    public required PublishCommit Commit { get; init; }

    /// <summary>
    /// Why it is not pressable, empty where it is - and also empty where the settings themselves
    /// are what blocks it, because the preflight list beside the button already carries every one
    /// of those in the backend's own words. A second sentence repeating them would be this module
    /// paraphrasing a diagnostic it did not write.
    /// </summary>
    public required string Blocked { get; init; }

    public bool IsBlocked => Blocked.Length > 0;

    /// <summary>
    /// The gate before anything has been read: nothing is committable, and there is nothing to
    /// say about it yet. It is a state rather than a gap, the same way an unresolved form is.
    ///
    /// Its commit is the one an unread running state yields, taken from the same derivation
    /// every other reading goes through rather than written down a second time here.
    /// </summary>
    public static readonly PublishGate Unread =
        new() { CanStartSharing = false, Commit = CommitFor(null), Blocked = "" };

    /// <summary>
    /// The gate for one reading of everything the commit depends on.
    /// </summary>
    /// <param name="publishable">The form's own answer about the settings, false before one arrives.</param>
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

        // What the caller is entitled to assume, stated where it is produced rather than where
        // it is relied on: the review draws its label off this and the flow sends the effect it
        // names, and neither is in a position to notice the two coming apart.
        Assert.That(
            (gate.Commit == PublishCommit.Apply) == (publish?.Live is not null),
            "a commit that applies has a stream to apply to", gate.Commit);

        return gate;
    }

    /// <summary>
    /// Which of the two effects the commit is, for one reading of what is publishing.
    ///
    /// <b>The rule lives here alone because two sides read it.</b> The render pass draws the
    /// label and the sentence off the gate this composes, and the press reads what is
    /// publishing again and sends accordingly - a press that trusted the gate the last pass
    /// composed would be acting on a stream that may have started or ended since, and the
    /// backend refuses each of the two effects in exactly the state the other one is for.
    ///
    /// Null is a running state nothing has read yet, which is not a live stream: the honest
    /// reading of a state nobody has established is that no pipeline is in force, and the
    /// commit is locked for other reasons at that point anyway.
    /// </summary>
    public static PublishCommit CommitFor(PublishState? publish)
        => publish?.Live is not null ? PublishCommit.Apply : PublishCommit.Start;

    /// <summary>
    /// The first condition that stands in the way, in the order the reader can act on them: a
    /// backend that cannot be reached at all, then a relay with nothing to send to. Only one is
    /// shown, because a reader fixes them in this order anyway and two sentences would not say
    /// more than the first.
    ///
    /// A stream already on the air used to sit between them and does not any more. Refusing the
    /// commit for it would now be this module standing in front of a call that would succeed:
    /// <see cref="PublishCommit.Apply"/> is the effect for precisely that state, and the reader
    /// no longer has to walk to another screen and stop a stream to change one setting on it.
    /// </summary>
    private static string BlockedBy(string unreachable, RelayStatus? relay)
    {
        if (unreachable.Length > 0)
        {
            return unreachable;
        }

        // Null is not "unreachable": nothing has asked the relay yet, and a shell that read that
        // as a failure would name a condition nobody has established.
        if (relay is null)
        {
            return "Reading what the relay is carrying.";
        }

        if (!relay.Reachable)
        {
            // The relay's own words where it gave any. The zero snapshot the backend starts on
            // carries none, which is a relay nothing has reached rather than one that refused.
            return relay.Error.Length > 0
                ? relay.Error
                : "The relay could not be reached, so there is nothing to publish to.";
        }

        return "";
    }
}
