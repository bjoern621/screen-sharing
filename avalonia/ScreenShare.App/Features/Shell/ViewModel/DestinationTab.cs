using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;

namespace ScreenShare.App.Features.Shell.ViewModel;

/// <summary>
/// One segment of the destination control.
/// Carries the destination rather than an index, so a selection is a value the shell acts on directly and no
/// screen holds a position in the strip.
///
/// Nothing here says whether the segment can be pressed: every destination is reachable at all times, and
/// a flag that is true for the tab's whole life is a fact no widget has to be told
/// (<c>docs/design-language.md</c>, "Surfaces and shape").
/// </summary>
public sealed class DestinationTab
{
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
}
