using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The pointer the publish sends beside the picture, as the preview draws it.
///
/// The position arrives in the picture's own pixels and is drawn in the card's, so the one
/// thing worth locking is the conversion: a marker placed in the publisher's pixels would sit
/// wherever the card happened not to be.
/// </summary>
public sealed class PointerTests
{
    /// <summary>A card drawn at the given size, with no picture behind it yet.</summary>
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
    /// Nothing is drawn while the publish sends no position, which is every cursor mode but the
    /// one that sends it. A marker drawn anyway would be a second pointer over the one the
    /// encoder already put in the frames.
    /// </summary>
    [Fact]
    public void NoPositionDrawsNoMarker()
    {
        var card = Card(800, 450);

        card.Point(null);

        Assert.False(card.HasPointer);
    }

    /// <summary>
    /// A pointer that has left the captured screen is not at its last position, so it is not
    /// drawn there: a marker stuck against an edge for as long as the mouse is away is worse
    /// than none.
    /// </summary>
    [Fact]
    public void APointerOffTheScreenDrawsNoMarker()
    {
        var card = Card(800, 450);

        card.Point(new PointerPosition { X = 10, Y = 10, Visible = false });

        Assert.False(card.HasPointer);
    }

    /// <summary>
    /// A card that has not been laid out yet draws nothing, because there is no size to place a
    /// position against. It is the same answer as no position: what is missing is one of the two
    /// halves the conversion needs.
    /// </summary>
    [Fact]
    public void ACardWithNoSizeDrawsNoMarker()
    {
        var card = Card(0, 0);

        card.Point(new PointerPosition { X = 10, Y = 10, Visible = true });

        Assert.False(card.HasPointer);
    }
}
