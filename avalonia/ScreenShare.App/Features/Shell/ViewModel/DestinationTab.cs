using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.ViewModel;

/// <summary>
/// One segment of the destination control.
/// It carries the destination it stands for rather than an index, so a selection is a value the shell can act
/// on directly and no screen has to know that Broadcast is the middle one.
///
/// <see cref="IsAvailable"/> is the whole of the unavailable treatment.
/// The design dims an unreachable destination and does nothing else to it - no strike, no badge, no hiding -
/// because a missing destination reads as a bug, and a dimmed one teaches the app's shape.
/// </summary>
public sealed class DestinationTab : Observable
{
    private bool _isAvailable = true;

    public DestinationTab(Destination value)
    {
        Value = value;
        Label = Destinations.LabelOf(value);

        Assert.That(Label.Length > 0, "a segment carries the label its destination is named by", (int)value);
    }

    /// <summary>The destination this segment selects. Fixed for the tab's life.</summary>
    public Destination Value { get; }

    /// <summary>Verbatim, from the destination table. Fixed for the tab's life.</summary>
    public string Label { get; }

    /// <summary>
    /// Whether the destination can be reached right now.
    /// Written only by <see cref="SetAvailable"/>, which the strip's render function calls on every pass.
    /// </summary>
    public bool IsAvailable { get => _isAvailable; private set => Set(ref _isAvailable, value); }

    /// <summary>
    /// The one write.
    /// Idempotent: setting the availability the tab already has notifies nothing, so a render pass over an
    /// unchanged strip moves no pixels.
    /// </summary>
    public void SetAvailable(bool available) => IsAvailable = available;
}
