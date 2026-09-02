using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Controls;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Where the body and its side column land,
/// in the two arrangements one window can put them in (<c>docs/design-language.md</c>, "Narrow windows").
///
/// Defect locked out: a body measured against the whole window while a column stands in part of it,
/// which draws a card the reader cannot see the right-hand end of.
/// </summary>
public sealed class SideColumnPanelTests
{
    private const double Width = 1000;
    private const double Height = 600;
    private const double Side = 330;
    private const double Gap = 12;

    private static Control Region() => new Border();

    private static SideColumnPanel Laid(bool beside, bool sideOnLeft, Control body, Control side, Control? foot = null)
    {
        var panel = new SideColumnPanel
        {
            Beside = beside,
            SideOnLeft = sideOnLeft,
            SideWidth = Side,
            Gap = Gap,
        };

        panel.Children.Add(body);
        panel.Children.Add(side);

        if (foot is not null)
        {
            panel.Children.Add(foot);
        }

        panel.Measure(new Size(Width, Height));
        panel.Arrange(new Rect(0, 0, Width, Height));
        return panel;
    }

    [Fact]
    public void AColumnOnTheRightLeavesTheBodyWhatIsLeftOfTheWindow()
    {
        var body = Region();
        var side = Region();
        Laid(beside: true, sideOnLeft: false, body, side);

        Assert.Equal(new Rect(0, 0, Width - Side - Gap, Height), body.Bounds);
        Assert.Equal(new Rect(Width - Side, 0, Side, Height), side.Bounds);
    }

    [Fact]
    public void AColumnOnTheLeftStandsAtTheEdgeAndTheBodyFollowsIt()
    {
        var body = Region();
        var side = Region();
        Laid(beside: true, sideOnLeft: true, body, side);

        Assert.Equal(new Rect(Side + Gap, 0, Width - Side - Gap, Height), body.Bounds);
        Assert.Equal(new Rect(0, 0, Side, Height), side.Bounds);
    }

    /// <summary>Over the body, the panel is the window's width and the body keeps its own.</summary>
    [Fact]
    public void APanelOverTheBodyTakesTheWholeWindow()
    {
        var body = Region();
        var side = Region();
        Laid(beside: false, sideOnLeft: false, body, side);

        Assert.Equal(new Rect(0, 0, Width, Height), body.Bounds);
        Assert.Equal(new Rect(0, 0, Width, Height), side.Bounds);
    }

    /// <summary>A column that is not drawn takes no width, on either arrangement.</summary>
    [Fact]
    public void AHiddenColumnLeavesTheWholeWindowToTheBody()
    {
        var body = Region();
        var side = Region();
        side.IsVisible = false;
        Laid(beside: true, sideOnLeft: false, body, side);

        Assert.Equal(new Rect(0, 0, Width, Height), body.Bounds);
    }

    [Fact]
    public void AHiddenColumnOverTheBodyLeavesItTheWholeWindowToo()
    {
        var body = Region();
        var side = Region();
        side.IsVisible = false;
        Laid(beside: false, sideOnLeft: false, body, side);

        Assert.Equal(new Rect(0, 0, Width, Height), body.Bounds);
    }

    /// <summary>The foot the column carries stands under it, and the body keeps the whole height.</summary>
    [Fact]
    public void AFootStandsUnderTheColumnBesideTheBody()
    {
        var body = Region();
        var side = Region();
        var foot = new Border { Height = 40 };
        Laid(beside: true, sideOnLeft: false, body, side, foot);

        Assert.Equal(new Rect(Width - Side, Height - 40, Side, 40), foot.Bounds);
        Assert.Equal(Height - 40, side.Bounds.Height);
        Assert.Equal(Height, body.Bounds.Height);
    }

    /// <summary>
    /// Under a panel over the body, the foot crosses the window and the panel stops above it.
    /// The row the panel must not cover: an opened panel otherwise takes away the way on.
    /// </summary>
    [Fact]
    public void AFootUnderAPanelOverTheBodyCrossesTheWindow()
    {
        var body = Region();
        var side = Region();
        var foot = new Border { Height = 40 };
        Laid(beside: false, sideOnLeft: false, body, side, foot);

        Assert.Equal(new Rect(0, Height - 40, Width, 40), foot.Bounds);
        Assert.Equal(Height - 40, side.Bounds.Height);
        Assert.Equal(Height - 40, body.Bounds.Height);
    }

    /// <summary>A foot beside no column travels with the body, which then stops above it.</summary>
    [Fact]
    public void AFootWithNoColumnDrawnCrossesTheWindowToo()
    {
        var body = Region();
        var side = Region();
        side.IsVisible = false;
        var foot = new Border { Height = 40 };
        Laid(beside: true, sideOnLeft: false, body, side, foot);

        Assert.Equal(new Rect(0, Height - 40, Width, 40), foot.Bounds);
        Assert.Equal(new Rect(0, 0, Width, Height - 40), body.Bounds);
    }

    /// <summary>
    /// A window narrower than the column it was told to draw beside the body.
    /// The measured pass keeps the body at a width rather than a negative one, the arrangement holding either way.
    /// </summary>
    [Fact]
    public void AWindowNarrowerThanTheColumnLeavesTheBodyNoWidth()
    {
        var panel = new SideColumnPanel { Beside = true, SideWidth = Side, Gap = Gap };
        var body = Region();
        var side = Region();
        panel.Children.Add(body);
        panel.Children.Add(side);

        panel.Measure(new Size(200, Height));
        panel.Arrange(new Rect(0, 0, 200, Height));

        Assert.Equal(0, body.Bounds.Width);
        Assert.Equal(200, side.Bounds.Width);
    }
}
