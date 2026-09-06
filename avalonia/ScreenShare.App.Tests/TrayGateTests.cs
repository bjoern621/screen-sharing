using ScreenShare.App.Features.Tray.View;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The switch in front of the tray icon.
/// <c>MIRRORME_TRAY=0</c> registers no icon for the whole run,
/// which leaves the shell the lifetime a platform serving no tray already gets: quit on close.
/// </summary>
public sealed class TrayGateTests
{
    [Fact]
    public void ZeroRegistersNoIcon()
    {
        Assert.False(TrayIconHost.IsWanted("0"));
    }

    [Fact]
    public void EveryOtherValueRegistersOne()
    {
        Assert.True(TrayIconHost.IsWanted(null));
        Assert.True(TrayIconHost.IsWanted(""));
        Assert.True(TrayIconHost.IsWanted("1"));
        Assert.True(TrayIconHost.IsWanted("false"));
    }
}
