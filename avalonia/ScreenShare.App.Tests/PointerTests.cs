using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Insights.Preview.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Pointer the publish sends beside the picture, as the preview draws it.
/// A position arrives as a fraction of the picture and is drawn in the card's own pixels,
/// so the conversion is worth locking: a marker placed in anything else lands wherever the card is not.
/// </summary>
public sealed class PointerTests
{
    private static PreviewViewModel Card(double width, double height)
    {
        var backend = new SeededBackend("linux");
        var session = new Session(backend, static action => action());
        var form = new FormSession(backend, session, static action => action());
        var card = new PreviewViewModel(backend, form, session, static action => action());
        card.SetPictureSize(width, height);
        return card;
    }

    /// <summary>
    /// Every cursor mode but one sends no position.
    /// A marker drawn without one would be a second pointer over the one already coded into the frames.
    /// </summary>
    [Fact]
    public void NoPositionDrawsNoMarker()
    {
        var card = Card(800, 450);

        card.Point(null);

        Assert.False(card.HasPointer);
    }

    /// <summary>
    /// A fraction of the picture is drawn at that fraction of the card, and the marker's tip is what lands
    /// on it: the shape points at its own origin, so an offset here would place the tip beside the position.
    /// </summary>
    [Fact]
    public void APositionIsDrawnAtItsFractionOfTheCard()
    {
        var card = Card(800, 400);

        card.Point(new PointerPosition { X = 0.25f, Y = 0.75f, Visible = true });

        Assert.True(card.HasPointer);
        Assert.Equal(200, card.PointerLeft, 3);
        Assert.Equal(300, card.PointerTop, 3);
    }

    /// <summary>
    /// A pointer off the captured screen is not where it last was.
    /// A marker parked at an edge for as long as the mouse is away is worse than none.
    /// </summary>
    [Fact]
    public void APointerOffTheScreenDrawsNoMarker()
    {
        var card = Card(800, 450);

        card.Point(new PointerPosition { X = 0.5f, Y = 0.5f, Visible = false });

        Assert.False(card.HasPointer);
    }

    /// <summary>
    /// A card before its first layout has no size to place a position against.
    /// One half of the conversion is missing either way, so the answer is the one a missing position gets.
    /// </summary>
    [Fact]
    public void ACardWithNoSizeDrawsNoMarker()
    {
        var card = Card(0, 0);

        card.Point(new PointerPosition { X = 0.5f, Y = 0.5f, Visible = true });

        Assert.False(card.HasPointer);
    }
}
