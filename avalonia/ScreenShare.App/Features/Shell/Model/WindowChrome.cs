namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Who draws the window's frame.
///
/// It is one fact read in three places - whether the client area extends over the caption,
/// whether the caption the platform then asks the theme for is emptied out, and whether the
/// shell draws a title band of its own - so it is stated once here rather than answered three
/// times (docs/development-principles.md).
///
/// The app draws its own caption where the desktop has one caption to stand in for. Windows
/// and macOS each do: the shapes, their measurements and which corner they sit in are the same
/// on every machine running that system, so a band drawn to match is read as that system's
/// window rather than as this app's idea of one (docs/design-language.md, "Icons").
///
/// Linux has no such answer. Which buttons a window carries, which edge they sit on and
/// whether it carries any at all are the desktop's, and a tiling session answers "none" - so
/// an app-drawn caption there is the one window on the screen that came from somewhere else,
/// and no single set of shapes could be the right one for all of them. The frame is handed
/// back to the desktop instead, which draws whatever that desktop draws, including nothing.
/// </summary>
internal static class WindowChrome
{
    /// <summary>
    /// True where this app draws the caption, false where the desktop is left to do it.
    /// </summary>
    public static bool AppDrawsCaption => !OperatingSystem.IsLinux();
}
