using ScreenShare.App.Features.Shell.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Which exclusion a system gets.
/// Neither implementation holds state a test can read: Windows is a user32 call, every other system does
/// nothing.
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
