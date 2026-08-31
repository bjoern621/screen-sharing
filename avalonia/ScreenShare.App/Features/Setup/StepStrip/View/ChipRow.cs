using Avalonia;
using Avalonia.Controls;
using Avalonia.Layout;
using ScreenShare.App.Features.Setup.StepStrip.Model;

namespace ScreenShare.App.Features.Setup.StepStrip.View;

/// <summary>
/// Chips side by side, each holding an equal share of the row.
///
/// Computes nothing.
/// Every boundary comes from <see cref="ChipRowLayout"/>, pure and tested without a window,
/// so what is here is the two Avalonia passes.
/// </summary>
public sealed class ChipRow : Panel
{
    protected override Size MeasureOverride(Size availableSize)
    {
        if (Children.Count == 0)
        {
            return default;
        }

        var share = availableSize.Width / Children.Count;
        var width = 0.0;
        var height = 0.0;

        foreach (var child in Children)
        {
            child.Measure(new Size(share, availableSize.Height));
            width = Math.Max(width, child.DesiredSize.Width);
            height = Math.Max(height, child.DesiredSize.Height);
        }

        return new Size(width * Children.Count, height);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var edges = ChipRowLayout.Edges(finalSize.Width, Children.Count, LayoutHelper.GetLayoutScale(this));

        for (var i = 0; i < Children.Count; i++)
        {
            Children[i].Arrange(new Rect(edges[i], 0, edges[i + 1] - edges[i], finalSize.Height));
        }

        return finalSize;
    }
}
