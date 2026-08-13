using ScreenShare.App.Features.Shell.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Which mechanism keeps this app's windows out of a screen capture.
///
/// The defect this locks out is a system being handed a mechanism it does not have.
/// The Windows implementation is a `user32` call and the systems without one are answered by doing nothing,
/// so the selector is what decides whether a window is excluded or is left in the picture, and there is no
/// state on either implementation for a test to reach instead.
/// </summary>
public sealed class CaptureExclusionTests
{
    [Fact]
    public void WindowsStatesADisplayAffinityAndNoOtherSystemStatesAnything()
    {
        var exclusion = CaptureExclusions.ForThisSystem();

        Assert.Equal(OperatingSystem.IsWindows(), exclusion is DisplayAffinityExclusion);
        Assert.Equal(!OperatingSystem.IsWindows(), exclusion is NoCaptureExclusion);
    }
}
