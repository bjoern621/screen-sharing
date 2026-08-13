using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The answer for a system that cannot keep a window out of a capture, which is every one here except
/// Windows.
///
/// X11 serves the root window to any client that asks for it, so a capture there is a read of the whole
/// screen and there is no per-window fact to set.
/// Wayland carries no such request in any protocol, upstream or per desktop.
/// The compositors that can black a window out of a screencopy at all - Hyprland and niri among them - do it
/// from a window rule in their own configuration, which is the user's to write and not this app's to ask for;
/// and the Avalonia Wayland backend sends no `xdg_toplevel.set_app_id`, so a rule keyed on the application id
/// matches this window in an X11 session alone (`avalonia/README.md`).
///
/// So a tile drawing the screen it is itself drawn on nests indefinitely here, and this says so by doing
/// nothing rather than by hiding the picture: what a reader sees is what the stream is carrying.
/// </summary>
internal sealed class NoCaptureExclusion : ICaptureExclusion
{
    /// <inheritdoc />
    public void Exclude(Window window)
    {
    }
}
