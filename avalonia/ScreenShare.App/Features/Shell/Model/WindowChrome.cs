namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Which side draws the window's frame.
///
/// One fact with several readers: whether the client area extends over the caption, whether the caption the
/// platform then asks the theme for is emptied out, and whether the shell draws a title band of its own.
/// Stated once here rather than answered per site (docs/development-principles.md).
///
/// This app draws a caption where the desktop has one caption to stand in for.
/// Windows and macOS each have one: the shapes, their measurements and the corner they sit in are the same on
/// every machine running that system, so a band drawn to match reads as that system's window rather than as
/// this app's idea of one (docs/design-language.md, "Icons").
///
/// Linux has no single answer.
/// Which buttons a window carries, which edge they sit on and whether it carries any at all are the desktop's,
/// and a tiling session answers none, so an app-drawn caption there is the one window on the screen that came
/// from somewhere else and no single set of shapes fits every desktop.
/// The frame goes back to the desktop instead, which draws whatever it draws, including nothing.
/// </summary>
internal static class WindowChrome
{
    public static bool AppDrawsCaption => !OperatingSystem.IsLinux();
}
