using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.Model;

/// <summary>
/// Whether the commit can be pressed, and the one sentence saying why it cannot.
///
/// <b>Every condition here is a whole state some other side stated, read rather than
/// evaluated.</b> Whether the settings themselves publish is <c>Form.publishable</c>, which is
/// the backend's own answer and the same one that would refuse the call; whether a stream is
/// already in force is the presence of <c>PublishState.live</c>; whether the relay answered is
/// <c>RelayStatus.reachable</c>, and the reason it did not is the relay's own error text. None
/// of it is ranked, derived or re-decided here (<c>docs/ipc-api.md</c>, "The rule").
///
/// It is a record so that a render pass over unchanged state produces a value that compares
/// equal, which is what keeps <see cref="ReviewStep.ViewModel.ReviewStepViewModel.Apply"/>
/// idempotent.
/// </summary>
public sealed record PublishGate
{
    /// <summary>Whether the one red button is pressable.</summary>
    public required bool CanGoLive { get; init; }

    /// <summary>
    /// Why it is not, empty where it is - and also empty where the settings themselves are what
    /// blocks it, because the preflight list beside the button already carries every one of
    /// those in the backend's own words. A second sentence repeating them would be this module
    /// paraphrasing a diagnostic it did not write.
    /// </summary>
    public required string Blocked { get; init; }

    public bool IsBlocked => Blocked.Length > 0;

    /// <summary>
    /// The gate before anything has been read: nothing is committable, and there is nothing to
    /// say about it yet. It is a state rather than a gap, the same way an unresolved form is.
    /// </summary>
    public static readonly PublishGate Unread = new() { CanGoLive = false, Blocked = "" };

    /// <summary>
    /// The gate for one reading of everything the commit depends on.
    /// </summary>
    /// <param name="publishable">The form's own answer about the settings, false before one arrives.</param>
    /// <param name="unreachable">Why the backend could not describe the screen, empty while it can.</param>
    /// <param name="publish">What is publishing, null before the running state has been read.</param>
    /// <param name="relay">The relay snapshot, null before one has been read.</param>
    /// <param name="starting">Whether a start this flow asked for is still in flight.</param>
    public static PublishGate Of(
        bool publishable, string unreachable, PublishState? publish, RelayStatus? relay, bool starting)
    {
        Assert.NotNull(unreachable, "the gate reads the backend's own sentence, or the empty one");

        var blocked = BlockedBy(unreachable, publish, relay);

        return new PublishGate
        {
            CanGoLive = publishable && !starting && blocked.Length == 0,
            Blocked = blocked,
        };
    }

    /// <summary>
    /// The first condition that stands in the way, in the order the reader can act on them: a
    /// backend that cannot be reached at all, then a stream that is already on the air, then a
    /// relay with nothing to send to. Only one is shown, because a reader fixes them in this
    /// order anyway and three sentences would not say more than the first.
    /// </summary>
    private static string BlockedBy(string unreachable, PublishState? publish, RelayStatus? relay)
    {
        if (unreachable.Length > 0)
        {
            return unreachable;
        }

        if (publish?.Live is not null)
        {
            return "A stream is already publishing. Stop it on the broadcast screen before starting another.";
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
