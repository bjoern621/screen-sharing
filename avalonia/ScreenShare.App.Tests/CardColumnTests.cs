using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Controls;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The gap falls between the children that drew something.
/// The setup column's picker draws only on the step holding the screen setting,
/// and sits above the form on every step.
/// A stack charging a gap per visible child puts every other step's first card a gap below the rail beside it.
/// </summary>
public sealed class CardColumnTests
{
    private const double Gap = 12;

    private static CardColumn Column(params Control[] children)
    {
        var column = new CardColumn { Spacing = Gap };
        foreach (var child in children)
        {
            column.Children.Add(child);
        }

        return column;
    }

    private static Control Card(double height) => new Border { Height = height, Width = 100 };

    /// <summary>A child rendering nothing draws nothing, whatever its own visibility says.</summary>
    private static Control Empty() => new Border { Child = new Border { IsVisible = false } };

    private static CardColumn Laid(params Control[] children)
    {
        var column = Column(children);
        column.Measure(new Size(400, 400));
        column.Arrange(new Rect(0, 0, 400, column.DesiredSize.Height));
        return column;
    }

    [Fact]
    public void AChildThatDrewNothingTakesNoGap()
    {
        var card = Card(40);
        var column = Laid(Empty(), card);

        Assert.Equal(0, card.Bounds.Y);
        Assert.Equal(40, column.DesiredSize.Height);
    }

    [Fact]
    public void TwoDrawnChildrenAreOneGapApart()
    {
        var first = Card(40);
        var second = Card(30);
        var column = Laid(first, second);

        Assert.Equal(0, first.Bounds.Y);
        Assert.Equal(40 + Gap, second.Bounds.Y);
        Assert.Equal(40 + Gap + 30, column.DesiredSize.Height);
    }

    [Fact]
    public void AnEmptyChildBetweenTwoCardsLeavesOneGap()
    {
        var first = Card(40);
        var second = Card(30);
        Laid(first, Empty(), second);

        Assert.Equal(40 + Gap, second.Bounds.Y);
    }

    [Fact]
    public void AHiddenChildIsNoGapEither()
    {
        var hidden = Card(40);
        hidden.IsVisible = false;
        var card = Card(30);
        var column = Laid(hidden, card);

        Assert.Equal(0, card.Bounds.Y);
        Assert.Equal(30, column.DesiredSize.Height);
    }

    /// <summary>The one child a step draws starts at the top, the line the rail's first card is on.</summary>
    [Fact]
    public void TheOnlyDrawnChildStartsAtTheTop()
    {
        var card = Card(50);
        Laid(Empty(), Empty(), card, Empty());

        Assert.Equal(0, card.Bounds.Y);
    }
}
