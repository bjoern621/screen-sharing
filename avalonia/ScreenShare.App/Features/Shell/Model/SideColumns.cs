namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Every side column this app draws, with the body each needs beside it.
/// One table read by the three destinations and by the tests, so a width is stated where the column is described
/// and nowhere else.
/// </summary>
public static class SideColumns
{
    /// <summary>
    /// Rail pricing the settings beside them: cost, checks and presets
    /// (<c>Features/Setup/CostRail</c>).
    /// The body is a step's form, whose controls sit under labels that wrap.
    /// </summary>
    public static readonly SideColumn SetupRail = new(330, 470, ColumnEdge.Right, DrawnUnasked: true);

    /// <summary>
    /// Preview, the configuration read-back and the test streams, on the left of the live figures
    /// (<c>Features/Insights</c>).
    /// The body is the plots, the viewer table and the session log, each of which is a row of figures.
    /// </summary>
    public static readonly SideColumn InsightsCards = new(396, 444, ColumnEdge.Left, DrawnUnasked: true);

    /// <summary>
    /// Live actions in the insights header, beside the figures they act on.
    /// Under them on a narrower window: a band whose actions take the row leaves the figures a column one figure
    /// wide, and the header then runs deeper than the screen it heads.
    /// </summary>
    public static readonly SideColumn InsightsActions = new(440, 300, ColumnEdge.Right, DrawnUnasked: true);

    /// <summary>
    /// Watch settings, opened from the viewer's rail.
    /// The body it measures against is the tile grid and the stream rail together, both standing on the same
    /// window: measured against the grid alone, two side columns would leave the tiles a strip between them.
    /// </summary>
    public static readonly SideColumn ViewerWatch = new(330, 540, ColumnEdge.Right, DrawnUnasked: false);

    /// <summary>
    /// Stream rail carrying the names.
    /// Below its threshold the rail keeps its entries and drops the names, which is what the reader's own collapse
    /// does (<c>Features/Viewer/ViewModel/ViewerViewModel.cs</c>).
    /// </summary>
    public static readonly SideColumn ViewerRail = new(240, 300, ColumnEdge.Left, DrawnUnasked: true);
}
