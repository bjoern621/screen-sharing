namespace ScreenShare.App.Features.Setup.RegionPicker.Model;

/// <summary>
/// Words the region chooser owns, past the ones the form control brings with it.
/// Its heading and help come from the share group (<c>Copy/Fields.cs</c>), so nothing here repeats them.
/// </summary>
public static class RegionCopy
{
    public const string Draw = "Draw a rectangle";

    public const string Redraw = "Draw a new rectangle";

    public const string DrawTip = "Drag a rectangle on any screen. Press Escape to keep the current one.";

    /// <summary>Before anything is drawn.</summary>
    public const string Nothing = "No rectangle drawn yet. Draw one to set what viewers see.";

    /// <summary>
    /// A rectangle no single screen holds, which one screen capture cannot show.
    /// The stream may still carry it, so this is about the picture and not about sharing.
    /// </summary>
    public const string NoScreen =
        "The rectangle crosses more than one screen, so there is no preview. Draw one inside a single screen to see it.";

    /// <summary>The rectangle as a reader reads one back: the size, then where it sits.</summary>
    public static string Region(int x, int y, int width, int height)
        => $"{width} × {height} at {x}, {y}";
}
