namespace ScreenShare.App.Features.Shell.Model;

/// <summary>Which edge of the body a side column stands on.</summary>
public enum ColumnEdge
{
    Left,
    Right,
}

/// <summary>
/// One surface's side column: how wide it stands, and how much window the body beside it needs.
/// A window carrying both draws both.
/// A narrower one draws the body alone and opens the column over it
/// (<c>docs/design-language.md</c>, "Narrow windows").
///
/// The threshold is the two figures added rather than a third number, so a column that changes width moves it.
/// </summary>
/// <param name="Width">Column's own width, in device-independent pixels.</param>
/// <param name="Body">Narrowest body this surface is read in, the same units.</param>
/// <param name="Edge">Side of the body the column stands on.</param>
/// <param name="DrawnUnasked">Whether the column draws wherever it fits, or waits to be opened.</param>
public readonly record struct SideColumn(double Width, double Body, ColumnEdge Edge, bool DrawnUnasked)
{
    /// <summary>Narrowest window drawing the column beside the body.</summary>
    public double Beside => Width + Body;

    public bool FitsBeside(double window) => window >= Beside;
}
