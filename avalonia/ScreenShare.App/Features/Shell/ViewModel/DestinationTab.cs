using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.ViewModel;

/// <summary>
/// One segment of the destination control.
/// It carries the destination rather than an index, so a selection is a value the shell acts on directly and
/// no screen holds a position in the strip.
///
/// <see cref="IsAvailable"/> is the whole of the unavailable treatment.
/// The design dims an unreachable destination and does nothing else to it, no strike, no badge and no hiding,
/// because a destination that disappears reads as a bug while a dimmed one teaches the app's shape.
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

    /// <summary>Fixed for the tab's life.</summary>
    public Destination Value { get; }

    /// <summary>Verbatim from the destination table. Fixed for the tab's life.</summary>
    public string Label { get; }

    /// <summary>
    /// Whether the destination can be reached.
    /// Written by <see cref="SetAvailable"/> alone, which the strip's render function calls on every pass.
    /// </summary>
    public bool IsAvailable { get => _isAvailable; private set => Set(ref _isAvailable, value); }

    /// <summary>
    /// The one write.
    /// Idempotent: the availability the tab already holds notifies nothing, so a pass over an unchanged strip
    /// moves no pixels.
    /// </summary>
    public void SetAvailable(bool available) => IsAvailable = available;
}
