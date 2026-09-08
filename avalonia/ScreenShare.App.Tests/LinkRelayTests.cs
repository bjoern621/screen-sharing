using ScreenShare.App.Backend;
using ScreenShare.App.Features.Shell.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A launch carrying a link, and the window already open it offers the link to.
/// Each test names an instance of its own, so a run never reaches the app running on this machine
/// (<c>Backend/ControlEndpoint.cs</c>).
/// That name is the process environment, so it runs alone (<see cref="ProcessEnvironment"/>).
/// </summary>
[Collection(ProcessEnvironment.Name)]
public sealed class LinkRelayTests : IDisposable
{
    private const string Link = "mirrorme://watch/G1/bob/monitor-0";

    /// <summary>How long a link is given to cross, generous against a loaded test machine.</summary>
    private static readonly TimeSpan Crossing = TimeSpan.FromSeconds(2);

    /// <summary>How long a link that must not cross is watched for.</summary>
    private static readonly TimeSpan Quiet = TimeSpan.FromMilliseconds(200);

    private readonly string? _held = Environment.GetEnvironmentVariable(ControlEndpoint.EnvInstance);

    public LinkRelayTests() => Environment.SetEnvironmentVariable(
        ControlEndpoint.EnvInstance, "test-" + Guid.NewGuid().ToString("n"));

    public void Dispose()
    {
        Environment.SetEnvironmentVariable(ControlEndpoint.EnvInstance, _held);
        Environment.SetEnvironmentVariable(LinkRelay.EnvOneWindow, null);
    }

    /// <summary>Whether the link landed inside the span given.</summary>
    private static async Task<bool> LandedAsync(Task<string> landing, TimeSpan within)
        => landing == await Task.WhenAny(landing, Task.Delay(within)).ConfigureAwait(false);

    [Fact]
    public async Task AWindowTakesALinkALaterLaunchCarries()
    {
        var landed = new TaskCompletionSource<string>();
        LinkRelay.Listen(link => landed.TrySetResult(link));

        var handed = LinkRelay.TryHandOver(Link);

        Assert.True(handed, "a window that is listening takes the link");
        Assert.True(await LandedAsync(landed.Task, Crossing), "the link reaches the window");
        Assert.Equal(Link, await landed.Task);
    }

    [Fact]
    public void ALaunchWithNoWindowListeningKeepsTheLink()
        => Assert.False(LinkRelay.TryHandOver(Link), "a launch nothing answers draws its own window");

    [Fact]
    public async Task ASecondWindowLeavesTheLinksToTheFirst()
    {
        var first = new TaskCompletionSource<string>();
        var second = new TaskCompletionSource<string>();
        LinkRelay.Listen(link => first.TrySetResult(link));
        LinkRelay.Listen(link => second.TrySetResult(link));

        Assert.True(LinkRelay.TryHandOver(Link));

        Assert.True(await LandedAsync(first.Task, Crossing), "the window that took the endpoint takes the link");
        Assert.False(await LandedAsync(second.Task, Quiet), "one endpoint carries the links");
    }

    [Fact]
    public async Task AWindowTakesALaunchCarryingNoLink()
    {
        var landed = new TaskCompletionSource<string>();
        LinkRelay.Listen(link => landed.TrySetResult(link));

        var handed = LinkRelay.TryHandOver("");

        Assert.True(handed, "a window that is listening takes a launch carrying nothing");
        Assert.True(await LandedAsync(landed.Task, Crossing), "the launch reaches the window");
        Assert.Equal("", await landed.Task);
    }

    [Fact]
    public void ALaunchWithNoWindowListeningDrawsItsOwn()
        => Assert.False(LinkRelay.TryHandOver(""), "a launch nothing answers draws its own window");

    [Fact]
    public void AShellLeftOutOfTheHandOverTakesNoEndpoint()
    {
        Environment.SetEnvironmentVariable(LinkRelay.EnvOneWindow, "0");
        LinkRelay.Listen(_ => { });
        Environment.SetEnvironmentVariable(LinkRelay.EnvOneWindow, null);

        Assert.False(LinkRelay.TryHandOver(Link), "an endpoint nothing took answers no launch");
    }

    [Fact]
    public async Task AShellLeftOutOfTheHandOverOffersNoLaunch()
    {
        var landed = new TaskCompletionSource<string>();
        LinkRelay.Listen(link => landed.TrySetResult(link));

        Environment.SetEnvironmentVariable(LinkRelay.EnvOneWindow, "0");
        var handed = LinkRelay.TryHandOver(Link);
        Environment.SetEnvironmentVariable(LinkRelay.EnvOneWindow, null);

        Assert.False(handed, "a shell left out draws its own window");
        Assert.False(await LandedAsync(landed.Task, Quiet), "the window already open sees nothing of it");
    }
}
