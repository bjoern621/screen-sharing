using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.Nudge.ViewModel;

/// <summary>
/// The design's one editable control on this screen: smoother against sharper, drawn where a live-safe
/// quality change would belong if the backend had one.
///
/// <b>Inert, and the reason is on the control rather than in this comment.</b> No backend effect changes an
/// encoder's quality without rebuilding the pipeline: both engines run a child built from an argv and neither
/// takes a value back, so <c>ApplyToStream</c> restarts the stream and is the opposite of live-safe.
/// A slider wired to that is a control whose promise is false.
/// It stays on screen greyed and carrying why, the treatment the settings form gives a knob the combination
/// in force blocks (<c>docs/field-availability.md</c>, "The rule").
///
/// <see cref="IsEnabled"/> and <see cref="Reason"/> reach the screen only through their bindings, which
/// nothing here can assert.
/// The assertions at the foot of <see cref="Apply"/> are what a view model can do about it, and they keep the
/// two properties from disagreeing with each other.
///
/// The footnote reports the quality the stream <i>runs</i> at and predicts nothing from the slider.
/// What a new position costs is the encoder's answer, and a guess is a number a publisher acts on and is
/// wrong about.
/// </summary>
public sealed class NudgeViewModel : Observable
{
    /// <summary>Design's resting position, percent of the track.</summary>
    private const double DefaultSharpness = 46;

    public NudgeViewModel() => Apply();

    // --- Inputs, written from outside ----------------------------------------------

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
    /// Where the reader has put the slider: 0 smoother, 100 sharper.
    /// The reader's to own, so <see cref="Apply"/> never writes it.
    /// A render pass that moved the thumb would fight the hand holding it.
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

    // --- Outputs, written by Apply alone -------------------------------------------

    private string _estimate = "";

    /// <summary>Footnote under the slider: the quality in force, and what it costs.</summary>
    public string Estimate { get => _estimate; private set => Set(ref _estimate, value); }

    /// <summary>
    /// Whether the track takes a hand.
    /// Always false, and a property rather than a value in the markup, so the view binds one fact and the
    /// reason beside it cannot end up describing a live control.
    /// </summary>
    public bool IsEnabled => false;

    /// <summary>Why the track is inert, in place of an estimate the reader could act on.</summary>
    public string Reason => Copy.Cards.NudgeInert;

    /// <summary>
    /// Caveat beside the card's title.
    /// Read from the same table as <see cref="Reason"/>, so the short form and the long one cannot come to
    /// disagree.
    /// </summary>
    public string Caveat => Copy.Cards.NudgeCaveat;

    /// <summary>One render function.</summary>
    public void Apply()
    {
        var reading = Snapshot;

        Estimate = $"cq {Figure.Of(reading.Cq)} → measured {Figure.Of(reading.EgressMbps, "0.0")} Mb/s";

        Assert.That(Estimate.Length > 0, "a nudge always states the quality it is running at");
        Assert.That(!IsEnabled == (Reason.Length > 0), "an inert track says why", IsEnabled);
        Assert.That(!IsEnabled == (Caveat.Length > 0), "an inert track's title carries the caveat", IsEnabled);
    }
}
