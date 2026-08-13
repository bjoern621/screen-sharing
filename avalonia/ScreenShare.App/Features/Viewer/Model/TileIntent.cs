namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// What a tile asks the grid to do with it.
///
/// <b>A request and not a write.</b> Focus is at most one tile out of many, a pop-out moves a stream between
/// windows, and fullscreen is a property of a window - none of the three is a fact a single tile can know,
/// let alone set.
/// The tile raises the intent; the screen that owns the arrangement decides what it means for every other
/// tile (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
///
/// The first three are toggles, so the same intent twice returns to where it started rather than doing
/// something else the second time.
/// The last two name a state instead, because each arrives from something that means one direction: a key
/// that means "leave", and a window that has closed.
/// A toggle raised from either would ask for the state it was reporting the end of.
/// </summary>
public enum TileIntent
{
    /// <summary>Focus this tile, or give up focus when it already has it.</summary>
    Focus,

    /// <summary>Draw this stream in a window of its own, or return it to the grid.</summary>
    PopOut,

    /// <summary>Take the window drawing this stream fullscreen, or bring it back.</summary>
    Fullscreen,

    /// <summary>Give the window drawing this stream back to its grid, filling a screen or not.</summary>
    LeaveFullscreen,

    /// <summary>Draw this stream in the grid, whether or not it was in a window of its own.</summary>
    LeavePopOut,
}
