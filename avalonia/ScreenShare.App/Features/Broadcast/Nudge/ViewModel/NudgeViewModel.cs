using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.Nudge.ViewModel;

/// <summary>
/// The design's one editable control on this screen: smoother against sharper, drawn where a live-safe
/// quality change would belong if the backend had one.
///
/// <b>It is inert, and the reason is on the control rather than in this comment.</b> The backend has no
/// effect that changes an encoder's quality without rebuilding the pipeline: both engines run a child built
/// from an argv and neither takes a value back afterwards, so <c>ApplyToStream</c> restarts the stream and is
/// the opposite of live-safe.
/// A slider wired to that would be a control whose whole promise is false.
/// It stays on screen, greyed and carrying why, because the concept is one the reader is entitled to
/// understand - the same treatment the settings form gives a knob the current combination blocks
/// (<c>docs/field-availability.md</c>, "The rule").
///
/// <b>Greyed and carrying why are things the view has to draw, and for a while it drew neither.</b>
/// <see cref="IsEnabled"/> and <see cref="Reason"/> were stated here and bound nowhere, so the track took a
/// hand that changed nothing and the card's caption promised the live-safe apply this very comment denies.
/// Both are bound now, and the assertion at the foot of <see cref="Apply"/> is what a view model can do about
/// it: a state this one names has to reach the screen through a binding, and the invariant at least keeps the
/// two properties from disagreeing with each other.
///
/// The footnote reports the quality the stream is <i>running</i> at rather than predicting one from the
/// slider.
/// Nothing here knows what a new position would cost; the encoder answers that, and a guessed estimate would
/// be a number a publisher could act on and be wrong about.
/// </summary>
public sealed class NudgeViewModel : Observable
{
    /// <summary>The design's resting position, as a percentage of the track.</summary>
    private const double DefaultSharpness = 46;

    public NudgeViewModel() => Apply();

    // --- Inputs -------------------------------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;
    private double _sharpness = DefaultSharpness;

    public BroadcastSnapshot Snapshot
    {
        get => _snapshot;
        set
        {
            Assert.NotNull(value, "a nudge renders a reading");

            if (Set(ref _snapshot, value))
            {
                Apply();
            }
        }
    }

    /// <summary>
    /// Where the reader has put the slider, 0 for smoother and 100 for sharper.
    /// Owned by the reader, so <see cref="Apply"/> never writes it - a render pass that moved the thumb would
    /// fight the hand holding it.
    /// </summary>
    public double Sharpness
    {
        get => _sharpness;
        set
        {
            Assert.That(value is >= 0 and <= 100, "a nudge position is a percentage of the track", value);

            if (Set(ref _sharpness, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _estimate = "";

    /// <summary>The footnote under the slider: the quality in force, and what it is costing.</summary>
    public string Estimate { get => _estimate; private set => Set(ref _estimate, value); }

    /// <summary>
    /// Whether the track takes a hand.
    /// Always false, and stated as a property rather than written into the markup so the view has one thing
    /// to bind and the reason beside it cannot end up describing a control that is live.
    /// </summary>
    public bool IsEnabled => false;

    /// <summary>Why the track is inert, shown in place of an estimate the reader could act on.</summary>
    public string Reason => Copy.Cards.NudgeInert;

    /// <summary>
    /// The caveat beside the card's title.
    /// It sits where a permission slip would and says the opposite, because that is what is true: read from
    /// the same table as <see cref="Reason"/>, so the short form and the long one cannot come to disagree.
    /// </summary>
    public string Caveat => Copy.Cards.NudgeCaveat;

    /// <summary>The one render function.</summary>
    public void Apply()
    {
        var reading = Snapshot;

        Estimate = $"cq {Figure.Of(reading.Cq)} → measured {Figure.Of(reading.EgressMbps, "0.0")} Mb/s";

        Assert.That(Estimate.Length > 0, "a nudge always states the quality it is running at");
        Assert.That(!IsEnabled == (Reason.Length > 0), "an inert track says why", IsEnabled);
        Assert.That(!IsEnabled == (Caveat.Length > 0), "an inert track's title carries the caveat", IsEnabled);
    }
}
