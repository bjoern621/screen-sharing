using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Answer where the system cannot keep a window out of a capture, which is everything but Windows.
///
/// X11 serves the root window to any client that asks, so a capture is a read of the whole screen and there is no
/// per-window fact to set.
/// No Wayland protocol carries the request, upstream or per desktop.
/// The compositors that can black a window out of a screencopy at all, Hyprland and niri among them, do it from a
/// window rule in their own configuration, the user's to write rather than this app's to ask for, and the
/// Avalonia Wayland backend sends no <c>xdg_toplevel.set_app_id</c>, so such a rule matches this window in an X11
/// session alone (avalonia/README.md).
///
/// A tile drawing the screen it is itself drawn on therefore nests for as long as the stream runs, and this
/// says so by doing nothing rather than by hiding the picture: what a reader sees is what the stream carries.
/// </summary>
internal sealed class NoCaptureExclusion : ICaptureExclusion
{
    /// <inheritdoc />
    public void Exclude(Window window)
    {
    }
}
