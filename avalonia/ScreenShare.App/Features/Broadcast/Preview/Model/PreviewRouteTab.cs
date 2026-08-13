using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Preview.Model;

/// <summary>
/// One segment of the preview's route toggle.
/// It carries the route rather than an index, so a selection is a value the card acts on and no markup has to
/// know which segment end-to-end is.
///
/// The segmented control is a <c>ListBox</c> and a <c>ListBox</c> selects items, so an item that is the value
/// keeps the two-way binding free of a converter.
/// </summary>
public sealed class PreviewRouteTab
{
    public PreviewRouteTab(PreviewRoute value)
    {
        Value = value;
        Label = PreviewRoutes.LabelOf(value);

        Assert.That(Label.Length > 0, "a segment carries the label its route is named by", (int)value);
    }

    public PreviewRoute Value { get; }

    /// <summary>Taken from the route table, verbatim.</summary>
    public string Label { get; }
}
