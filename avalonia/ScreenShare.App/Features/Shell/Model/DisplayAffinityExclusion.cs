using System.Runtime.InteropServices;
using System.Runtime.Versioning;
using Avalonia.Controls;
using Avalonia.Logging;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The Windows exclusion: a display affinity the kernel stores against the window.
///
/// <c>WDA_EXCLUDEFROMCAPTURE</c> leaves the window on its monitor and takes it out of what the desktop window
/// manager composes for anything else, so a capture of that desktop carries the windows behind it and nothing
/// of this one.
/// That covers every way a stream from this machine is captured: desktop duplication and the graphics capture
/// API behind <c>ddagrab</c> and <c>d3d11screencapturesrc</c>, and the screen bit blit behind <c>gdigrab</c>.
///
/// The value arrived in Windows 10 version 2004, and an older Windows applies <c>WDA_MONITOR</c> in its place,
/// leaving the window in the capture as an empty rectangle.
/// The two pictures differ and both break the feedback loop, since an empty rectangle carries no copy of the
/// screen either.
///
/// The affinity belongs to one top-level window of this process, so every window the shell opens states it for
/// itself.
/// </summary>
[SupportedOSPlatform("windows")]
internal sealed class DisplayAffinityExclusion : ICaptureExclusion
{
    /// <summary><c>WDA_EXCLUDEFROMCAPTURE</c>: on its monitor, absent from everything else composed.</summary>
    private const uint ExcludeFromCapture = 0x00000011;

    /// <inheritdoc />
    public void Exclude(Window window)
    {
        Assert.NotNull(window, "an exclusion names the window it keeps out of captures");

        // The platform has made the handle by the time a window is open.
        // Excluding one that has none is this shell calling too early, a bug here rather than a state to
        // survive.
        var handle = Assert.NotNull(
            window.TryGetPlatformHandle(),
            "a window is excluded once the platform has given it a handle");

        if (SetWindowDisplayAffinity(handle.Handle, ExcludeFromCapture))
        {
            return;
        }

        // A session that refuses costs this window its exclusion and nothing else, so it is reported rather
        // than thrown.
        // The window is back in every capture, and the error code is the only thing saying why.
        Logger.TryGet(LogEventLevel.Warning, "CaptureExclusion")?.Log(
            this,
            "the window stays in screen captures: SetWindowDisplayAffinity failed with {Error}",
            Marshal.GetLastWin32Error());
    }

    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetWindowDisplayAffinity(IntPtr window, uint affinity);
}
