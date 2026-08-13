namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// What a tile asks the grid for.
///
/// A request and not a write: focus is at most one tile out of many, a pop-out moves a stream between
/// windows, and fullscreen is a property of a window.
/// The tile raises the intent, and the screen owning the arrangement decides what it means for the others
/// (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
///
/// An intent either toggles or names a state.
/// A leave intent names one, because it arrives from something that means a single direction, a key meaning
/// "leave" or a window that has closed, and a toggle raised from there would ask for the state it was
/// reporting the end of.
/// </summary>
public enum TileIntent
{
    /// <summary>Toggle: focus this tile, or give focus up where it already has it.</summary>
    Focus,

    /// <summary>Toggle: draw this stream in a window of its own, or return it to the grid.</summary>
    PopOut,

    /// <summary>Toggle: fill a screen with the window drawing this stream, or give the screen back.</summary>
    Fullscreen,

    /// <summary>Windowed again, whether or not it was filling a screen.</summary>
    LeaveFullscreen,

    /// <summary>Drawn in the grid, whether or not it was in a window of its own.</summary>
    LeavePopOut,
}
