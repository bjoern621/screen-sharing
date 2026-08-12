using System.Runtime.InteropServices;
using System.Runtime.Versioning;
using Avalonia.Controls;
using Avalonia.Logging;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Shell.Model;

/// <summary>
/// The Windows exclusion: a display affinity stored against the window in kernel mode.
///
/// <c>WDA_EXCLUDEFROMCAPTURE</c> leaves the window on the monitor and takes it out of what the
/// desktop window manager composes for anything else, so a capture of the desktop it sits on
/// carries the windows behind it and nothing of this one. That covers every way a stream from
/// this machine is captured: the desktop duplication and the graphics capture API behind
/// `ddagrab` and `d3d11screencapturesrc`, and the screen bit blit behind `gdigrab`.
///
/// The value arrived in Windows 10 version 2004, and an older Windows applies `WDA_MONITOR` in
/// its place, which leaves the window in the capture as an empty rectangle. The two pictures
/// differ and the feedback loop is broken by both, since an empty rectangle carries no copy of
/// the screen either.
///
/// The affinity is a property of one top-level window of this process, so each window the shell
/// opens states it for itself.
/// </summary>
[SupportedOSPlatform("windows")]
internal sealed class DisplayAffinityExclusion : ICaptureExclusion
{
    /// <summary>
    /// `WDA_EXCLUDEFROMCAPTURE`: the window is displayed on a monitor, and everywhere else it
    /// does not appear at all.
    /// </summary>
    private const uint ExcludeFromCapture = 0x00000011;

    /// <inheritdoc />
    public void Exclude(Window window)
    {
        Assert.NotNull(window, "an exclusion names the window it keeps out of captures");

        // The handle is the window's own and the platform has made it by the time a window is
        // open. A window asking to be excluded before it has one is this shell calling too
        // early, which is a bug here rather than a state to survive.
        var handle = Assert.NotNull(
            window.TryGetPlatformHandle(),
            "a window is excluded once the platform has given it a handle");

        if (SetWindowDisplayAffinity(handle.Handle, ExcludeFromCapture))
        {
            return;
        }

        // A session that refuses costs this window its exclusion and costs nothing else, so it
        // is reported rather than thrown. What it means is that the window is in the capture
        // again, and the error is the only thing that says why.
        Logger.TryGet(LogEventLevel.Warning, "CaptureExclusion")?.Log(
            this,
            "the window stays in screen captures: SetWindowDisplayAffinity failed with {Error}",
            Marshal.GetLastWin32Error());
    }

    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetWindowDisplayAffinity(IntPtr window, uint affinity);
}
