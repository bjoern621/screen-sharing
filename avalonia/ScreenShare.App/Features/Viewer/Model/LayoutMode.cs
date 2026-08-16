namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// How tiles sit relative to each other.
///
/// Which window a tile is drawn in is a separate fact, so a tile can be focused and also filling a second monitor.
/// Folding a window state in here would give one field two meanings.
///
/// Nothing about the arrangement crosses the control contract: the backend describes decodes, not tiles
/// (<c>docs/ipc-api.md</c>).
/// </summary>
public enum LayoutMode
{
    /// <summary>Equal tiles in rows, placed by <c>Features/Viewer/Model/TileLayout.cs</c>.</summary>
    Grid,

    /// <summary>
    /// One tile takes the space, the rest sit in a rail beneath it.
    /// At most one tile is focused, never several.
    /// Which one is held by stream name, so a stream that drops out and comes back returns to its place.
    /// </summary>
    Focus,
}
