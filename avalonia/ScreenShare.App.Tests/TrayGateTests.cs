using ScreenShare.App.Features.Tray.View;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The switch in front of the tray icon: the app setting, and the environment variable over it.
/// A run with no icon keeps the lifetime a platform serving no tray already gets: quit on close.
/// </summary>
public sealed class TrayGateTests
{
    [Fact]
    public void TheSettingDecidesWhereTheEnvironmentNamesNothing()
    {
        Assert.True(TrayIconHost.IsWanted(null, wanted: true));
        Assert.False(TrayIconHost.IsWanted(null, wanted: false));
    }

    /// <summary><c>MIRRORME_TRAY=0</c> keeps the icon out for a whole run, whatever the setting holds.</summary>
    [Fact]
    public void ZeroRegistersNoIcon()
    {
        Assert.False(TrayIconHost.IsWanted("0", wanted: true));
        Assert.False(TrayIconHost.IsWanted("0", wanted: false));
    }

    [Fact]
    public void EveryOtherValueLeavesTheSettingDeciding()
    {
        Assert.True(TrayIconHost.IsWanted("", wanted: true));
        Assert.True(TrayIconHost.IsWanted("1", wanted: true));
        Assert.True(TrayIconHost.IsWanted("false", wanted: true));
        Assert.False(TrayIconHost.IsWanted("1", wanted: false));
    }
}
