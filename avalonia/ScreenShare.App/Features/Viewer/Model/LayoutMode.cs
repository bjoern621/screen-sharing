namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// How the grid arranges what it is drawing.
///
/// <b>Two members, and fullscreen is deliberately not one of them.</b> A mode says how several tiles are
/// arranged relative to each other; fullscreen and pop-out say which window a tile is drawn in.
/// Folding a window state into this enum would give one field two meanings and make states like "focused, and
/// also popped out on the second monitor" unrepresentable - which is a state the reader can plainly ask for.
///
/// Nothing about this crosses the control contract.
/// The backend describes decodes, and a decode is not a tile: how a viewer arranges what it receives is this
/// shell's whole job (<c>docs/ipc-api.md</c>).
/// </summary>
public enum LayoutMode
{
    /// <summary>
    /// Every tile is equal, arranged in rows that fill the space
    /// (<c>Features/Viewer/Model/TileLayout.cs</c>).
    /// </summary>
    Grid,

    /// <summary>
    /// One tile takes the space and the rest sit in a rail beneath it.
    ///
    /// Exactly one is focused or none is, never several: "focus" that admits two is a grid with extra steps.
    /// Which one is the shell's own state, and it is named by stream so that a stream which drops out and
    /// comes back returns to the place the reader put it.
    /// </summary>
    Focus,
}
