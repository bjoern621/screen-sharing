using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// Keeps a window out of the picture a screen capture taken on this machine produces.
///
/// <b>What it is for.</b> A capture of the screen this app is drawn on contains this app's own windows, and a
/// tile in one of them is drawing that capture back.
/// Every round trip through the encoder and the decoder therefore nests one more copy of the screen inside
/// the picture, and the depth grows with how long the stream has been running rather than settling anywhere.
/// Nothing downstream can undo it: the nesting is already in the pixels the capture handed over, so the
/// window the capture sees is the only place it can be prevented.
///
/// <b>Why it is an interface.</b> Whether a window can be hidden from a capture at all is the windowing
/// system's answer, and the systems disagree completely.
/// Windows stores a display affinity against the window in kernel mode and honours it in the capture paths
/// the desktop window manager composes for.
/// X11 hands the root window to any client that asks, and nothing mediates the read.
/// No Wayland protocol carries the request, so what exists there is per compositor and is the user's
/// configuration rather than a call this app can make.
///
/// <b>It names a state and not a transition.</b> <see cref="Exclude"/> says what has to be true of the window
/// afterwards, so a second call over a window that is already out of captures changes nothing.
/// </summary>
internal interface ICaptureExclusion
{
    /// <summary>
    /// Keeps one top-level window out of every capture taken on this machine, where the system has a way to
    /// say it.
    /// </summary>
    void Exclude(Window window);
}

/// <summary>
/// Which exclusion answers for this system.
/// It is the one place a platform's mechanism is chosen, so a window that asks to stay out of captures names
/// no system and no API.
/// </summary>
internal static class CaptureExclusions
{
    /// <summary>
    /// The exclusion this system offers, and <see cref="NoCaptureExclusion"/> where it offers none.
    /// One per caller, since both implementations hold nothing.
    /// </summary>
    public static ICaptureExclusion ForThisSystem()
        => OperatingSystem.IsWindows() ? new DisplayAffinityExclusion() : new NoCaptureExclusion();
}
