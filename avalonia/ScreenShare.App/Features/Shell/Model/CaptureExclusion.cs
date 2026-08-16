using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Keeps a window out of the pictures screen captures on this machine produce.
///
/// <b>Why.</b> A capture of the screen this app is drawn on carries this app's own windows, and a tile in one
/// of them is drawing that capture back.
/// Each round trip through the encoder and the decoder nests one more copy of the screen inside the picture,
/// and the depth grows for as long as the stream runs.
/// Nothing downstream undoes it: the nesting is already in the pixels the capture handed over, so the window
/// the capture sees is the only place to stop it.
///
/// <b>Why an interface.</b> Whether a window can be hidden from a capture at all is the windowing system's answer.
/// Windows stores a display affinity against the window in kernel mode and honours it in the capture paths the
/// desktop window manager composes for.
/// X11 hands the root window to any client that asks, with nothing mediating the read.
/// No Wayland protocol carries the request, so what exists is per compositor and is the user's configuration
/// rather than a call.
///
/// <b>A state and not a transition.</b> <see cref="Exclude"/> names what has to be true of the window
/// afterwards, so a second call over a window already out of captures changes nothing.
/// </summary>
internal interface ICaptureExclusion
{
    /// <summary>
    /// Keeps one top-level window out of every capture on this machine, where the system says it at all.
    /// </summary>
    void Exclude(Window window);
}

/// <summary>
/// One place a platform's exclusion mechanism is chosen, so a window asking to stay out of captures names no
/// system and no API.
/// </summary>
internal static class CaptureExclusions
{
    /// <summary>
    /// This system's exclusion, <see cref="NoCaptureExclusion"/> where it has none.
    /// One per caller: both implementations hold nothing.
    /// </summary>
    public static ICaptureExclusion ForThisSystem()
        => OperatingSystem.IsWindows() ? new DisplayAffinityExclusion() : new NoCaptureExclusion();
}
