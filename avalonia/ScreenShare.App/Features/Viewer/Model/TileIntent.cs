namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// What a tile asks the grid to do with it.
///
/// <b>A request and not a write.</b> Focus is at most one tile out of many, a pop-out moves a
/// stream between windows, and fullscreen is a property of a window - none of the three is a fact
/// a single tile can know, let alone set. The tile raises the intent; the screen that owns the
/// arrangement decides what it means for every other tile
/// (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
///
/// Every one of them is a toggle, so the same intent twice returns to where it started rather
/// than doing something else the second time.
/// </summary>
public enum TileIntent
{
    /// <summary>Focus this tile, or give up focus when it already has it.</summary>
    Focus,

    /// <summary>Draw this stream in a window of its own, or return it to the grid.</summary>
    PopOut,

    /// <summary>Take the window drawing this stream fullscreen, or bring it back.</summary>
    Fullscreen,
}
