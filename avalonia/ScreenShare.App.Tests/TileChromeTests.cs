using ScreenShare.App.Features.Viewer.Tile.View;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A picture somebody watches loses its chrome and its cursor, and a panel somebody reads keeps both.
/// Asserted: which of the two a card is, off the pointer, the tile's overlay and the host's answer.
/// </summary>
public sealed class TileChromeTests
{
    [Fact]
    public void APointerHoldingStillOverAPictureClearsIt()
    {
        var chrome = TileChrome.Of(pointerResting: true, showStats: false, pictureElsewhere: false);

        Assert.False(chrome.Stats);
        Assert.True(chrome.Resting);
    }

    /// <summary>The cursor is what a reader points the figures out with, and the name says whose they are.</summary>
    [Fact]
    public void TheStatsOverlayHoldsTheChromeAndTheCursor()
    {
        var chrome = TileChrome.Of(pointerResting: true, showStats: true, pictureElsewhere: false);

        Assert.True(chrome.Stats);
        Assert.False(chrome.Resting);
    }

    [Fact]
    public void AStirredPointerDrawsTheChrome()
    {
        var chrome = TileChrome.Of(pointerResting: false, showStats: false, pictureElsewhere: false);

        Assert.False(chrome.Resting);
    }

    /// <summary>
    /// A stream popped out draws its figures in the window it went to,
    /// which leaves the slot behind a picture like any other.
    /// </summary>
    [Fact]
    public void ASlotWhosePictureMovedRestsWithTheOverlayOn()
    {
        var chrome = TileChrome.Of(pointerResting: true, showStats: true, pictureElsewhere: true);

        Assert.False(chrome.Stats);
        Assert.True(chrome.Resting);
    }
}
