using System.Runtime.ExceptionServices;
using ScreenShare.App.Features.Tray.View;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The filter for Avalonia's tray watch crash (AvaloniaUI/Avalonia#21979).
/// Disposing a DBus tray icon cancels its watch loop,
/// and the cancellation escapes an async void into the dispatcher,
/// aborting an orderly quit with exit code 134.
/// The host marks exactly that exception handled, matched by type and by the frame
/// that threw it; every other dispatcher exception still crashes, as a broken contract should.
/// </summary>
public sealed class TrayWatchTests
{
    private const string WatchFrame = "   at Avalonia.FreeDesktop.DBusTrayIconImpl.WatchAsync()";

    private static Exception OffTheWatch(Exception thrown)
    {
        ExceptionDispatchInfo.SetRemoteStackTrace(thrown, WatchFrame);
        return thrown;
    }

    [Fact]
    public void TheCancellationEscapingTheWatchIsRecognised()
    {
        Assert.True(TrayIconHost.IsWatchDisposeCancellation(OffTheWatch(new TaskCanceledException())));
        Assert.True(TrayIconHost.IsWatchDisposeCancellation(OffTheWatch(new OperationCanceledException())));
    }

    [Fact]
    public void ACancellationThrownAnywhereElsePassesThrough()
    {
        Assert.False(TrayIconHost.IsWatchDisposeCancellation(new TaskCanceledException()));
    }

    [Fact]
    public void AnotherExceptionOffTheWatchPassesThrough()
    {
        Assert.False(TrayIconHost.IsWatchDisposeCancellation(OffTheWatch(new InvalidOperationException())));
    }
}
