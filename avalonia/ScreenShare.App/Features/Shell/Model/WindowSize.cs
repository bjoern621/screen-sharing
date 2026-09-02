namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Sizes the window opens at, and the floor it states to the desktop.
///
/// The floor is the narrowest and shortest arrangement every screen is drawn for: one column, the side column
/// over the body, and every band wrapping rather than running past the edge
/// (<c>docs/design-language.md</c>, "Narrow windows").
/// A desktop that tiles hands a window whatever its layout leaves, a quarter of the screen among them, so the floor
/// is what a window can be asked for rather than what it is usually given.
/// </summary>
public static class WindowSize
{
    public const double Opening = 1240;

    public const double OpeningHeight = 760;

    public const double Floor = 380;

    public const double FloorHeight = 320;
}
