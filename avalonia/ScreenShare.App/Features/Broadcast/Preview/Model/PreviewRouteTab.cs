using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Preview.Model;

/// <summary>
/// One segment of the preview's route toggle. It carries the route it stands for rather than
/// an index, so a selection is a value the card can act on and no markup has to know that
/// end-to-end is the second one.
///
/// The same shape the destination strip's segment has, and for the same reason: the segmented
/// control is a <c>ListBox</c>, a <c>ListBox</c> selects items, and an item that is the value
/// is what keeps the two-way binding free of a converter.
/// </summary>
public sealed class PreviewRouteTab
{
    public PreviewRouteTab(PreviewRoute value)
    {
        Value = value;
        Label = PreviewRoutes.LabelOf(value);

        Assert.That(Label.Length > 0, "a segment carries the label its route is named by", (int)value);
    }

    /// <summary>The route this segment selects. Fixed for the tab's life.</summary>
    public PreviewRoute Value { get; }

    /// <summary>Verbatim, from the route table. Fixed for the tab's life.</summary>
    public string Label { get; }
}
