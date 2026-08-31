using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Relay's own player page, opened in the machine's browser.
/// What a browser does with an address is out of this shell's reach,
/// so asserted is the seam: legs off the catalog, the row asking for the leg pressed, and a call on every press.
/// </summary>
public sealed class ViewerBrowserTests
{
    private static (ViewerViewModel Viewer, SeededBackend Backend) Watching(string stream)
    {
        var backend = new SeededBackend("linux")
        {
            Relay = new RelayStatus { Reachable = true, Paths = { new RelayPath { Name = stream, OwnName = stream, Ready = true } } },
        };

        var session = new Session(backend, static action => action());
        session.Start();

        var viewer = Flows.Viewer(backend, session);
        viewer.Apply();

        return (viewer, backend);
    }

    /// <summary>
    /// A list held here would decide which protocols the relay serves a page for,
    /// which the contract forbids (<c>docs/ipc-api.md</c>).
    /// </summary>
    [Fact]
    public void TheBrowserLegsAreTheOnesTheBackendNamed()
    {
        var (viewer, _) = Watching("bob");

        var row = Assert.Single(viewer.Streams);
        Assert.Equal(SeededBackend.BrowserLegs, row.BrowserLegs.Select(leg => leg.Value));
    }

    /// <summary>
    /// Neither roster contains the other: no player opens WHEP, and a browser reaches neither SRT nor RTSP.
    /// One list for both menus would be the narrower of them, and a leg taken off a reader who can run it.
    /// </summary>
    [Fact]
    public void ThePlayerLegsAndTheBrowserLegsAreDifferentRosters()
    {
        var (viewer, _) = Watching("bob");

        var row = Assert.Single(viewer.Streams);
        var players = row.Legs.Select(leg => leg.Value).ToList();
        var browser = row.BrowserLegs.Select(leg => leg.Value).ToList();

        Assert.NotEmpty(players);
        Assert.NotEmpty(browser);
        Assert.Contains(browser, leg => !players.Contains(leg));
        Assert.Contains(players, leg => !browser.Contains(leg));
    }

    /// <summary>
    /// Stream and leg together are the identity, the relay re-serving one stream on all its listeners.
    /// </summary>
    [Fact]
    public void PressingALegAsksForThatPage()
    {
        var (viewer, backend) = Watching("bob");
        var row = Assert.Single(viewer.Streams);

        row.BrowserLegs.Single(leg => leg.Value == "webrtc").Open.Execute(null);

        var asked = Assert.Single(backend.Browsed);
        Assert.Equal("bob", asked.StreamName);
        Assert.Equal("webrtc", asked.Transport);
    }

    /// <summary>
    /// A tab belongs to the browser that opened it and this side cannot read whether it is still there,
    /// so no state exists a second press could find true.
    /// </summary>
    [Fact]
    public void ASecondPressAsksAgain()
    {
        var (viewer, backend) = Watching("bob");
        var leg = Assert.Single(viewer.Streams).BrowserLegs.Single(leg => leg.Value == "hls");

        leg.Open.Execute(null);
        leg.Open.Execute(null);

        Assert.Equal(2, backend.Browsed.Count);
    }

    /// <summary>
    /// <c>StartWatch</c> launches a viewer the backend supervises and reports.
    /// A page hands an address to a program neither side hears from again.
    /// </summary>
    [Fact]
    public void OpeningAPageLeavesTheWatchedSetAlone()
    {
        var (viewer, _) = Watching("bob");
        var row = Assert.Single(viewer.Streams);

        row.BrowserLegs.First().Open.Execute(null);

        Assert.False(row.IsWatched);
        Assert.DoesNotContain(row.Legs, leg => leg.IsOpen);
    }

    /// <summary>
    /// Legs are records so they compare equal, and the command is made once by the row that owns it.
    /// A command rebuilt per pass is a menu that loses a press to a poll.
    /// </summary>
    [Fact]
    public void ASecondPassLeavesTheLegsAsTheyWere()
    {
        var (viewer, _) = Watching("bob");
        var row = Assert.Single(viewer.Streams);
        var before = row.BrowserLegs.ToList();

        viewer.Apply();

        Assert.Equal(before, row.BrowserLegs);
        Assert.Same(before[0].Open, row.BrowserLegs[0].Open);
    }
}
