namespace ScreenShare.App.Features.Viewer.Tile.View;

/// <summary>
/// What a card puts over its picture, off the three facts that decide it.
///
/// One answer for the overlay and for the chrome, the overlay deciding the other:
/// figures on screen are what a pointer holding still is aimed at,
/// and clearing the picture there takes the cursor off the panel being read.
/// </summary>
/// <param name="Stats">Whether this card draws the stats overlay.</param>
/// <param name="Resting">Whether the chrome and the cursor come off.</param>
public readonly record struct TileChrome(bool Stats, bool Resting)
{
    /// <summary>
    /// Reads a card's state.
    /// The pointer is the card's own fact (<see cref="RestingWatch"/>), the overlay the tile's,
    /// and a picture drawn elsewhere the host's.
    /// </summary>
    public static TileChrome Of(bool pointerResting, bool showStats, bool pictureElsewhere)
    {
        // Figures belong to the card drawing the picture they describe,
        // so a slot whose stream popped out is a picture again and rests like one.
        var stats = showStats && !pictureElsewhere;

        return new TileChrome(stats, pointerResting && !stats);
    }
}
