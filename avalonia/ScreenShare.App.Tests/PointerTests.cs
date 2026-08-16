using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Pointer the publish sends beside the picture, as the preview draws it.
/// A position arrives in the captured screen's pixels and is drawn in the card's, so the conversion is what is
/// worth locking: a marker placed in the publisher's pixels lands wherever the card is not.
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
    /// A pointer off the captured screen is not where it last was.
    /// A marker parked at an edge for as long as the mouse is away is worse than none.
    /// </summary>
    [Fact]
    public void APointerOffTheScreenDrawsNoMarker()
    {
        var card = Card(800, 450);

        card.Point(new PointerPosition { X = 10, Y = 10, Visible = false });

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

        card.Point(new PointerPosition { X = 10, Y = 10, Visible = true });

        Assert.False(card.HasPointer);
    }
}
