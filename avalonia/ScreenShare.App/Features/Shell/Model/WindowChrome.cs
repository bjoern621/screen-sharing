using Avalonia;
using Avalonia.Controls;
using Avalonia.Styling;

using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Which side draws the window's frame.
///
/// One fact with several readers: whether the client area extends over the caption, whether the caption
/// the platform then asks the theme for is emptied out, and whether the shell draws a title band of its own.
/// Stated once here rather than answered per site (<c>docs/development-principles.md</c>).
///
/// This app draws a caption where the desktop has one caption to stand in for.
/// Windows and macOS each have one: the shapes, their measurements and the corner they sit in are the same
/// on every machine running that system, so a band drawn to match reads as that system's window rather than
/// as this app's idea of one (<c>docs/design-language.md</c>, "Icons").
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

    /// <summary>
    /// Puts the window's client area over the platform's caption and empties the replacement the platform then
    /// asks the theme for, so no window ends up with two (<c>Design/Windows.axaml</c>).
    /// Both writes name a state, so a second call changes nothing.
    ///
    /// Where the desktop draws the frame the window is left as it is.
    ///
    /// What stands in the caption's place is the window's own: the shell draws a band claiming the caption's
    /// hit-testing roles, and a stream's own window draws nothing and is dragged by its picture.
    ///
    /// Decorations stay full with the client area extended over them rather than dropping to a border.
    /// A frame without a caption loses what the Win32 compositor animates: no open, close, minimise or restore
    /// transition, and a top edge widened to the bare resize frame.
    /// Painting underneath keeps the platform's animations, snap and shadow.
    /// </summary>
    public static void PaintOverCaption(Window window)
    {
        Assert.NotNull(window, "a caption is painted over on a window");

        if (!AppDrawsCaption)
        {
            return;
        }

        // Off the application, where Design/Windows.axaml merges it:
        // a window is asked before it is shown, and an unrooted lookup walks no further than the window.
        var application = Assert.NotNull(Application.Current, "a window is shown by a running application");
        var decorations = Assert.NotNull(application.FindResource("EmptyDecorations") as ControlTheme,
            "the application's resources carry the empty window decorations theme");

        window.ExtendClientAreaToDecorationsHint = true;
        window.WindowDecorationsTheme = decorations;

        Assert.That(window.ExtendClientAreaToDecorationsHint, "the client area covers the caption");
    }
}
