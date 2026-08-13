using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The third way to watch: the relay's own player page, opened in the machine's browser.
///
/// What is asserted here is the seam rather than the page.
/// Nothing this shell can reach draws what a browser does with an address, so what these tests hold is the
/// part that is this side's: the legs come off the catalog and not off a list written here, the row asks for
/// the one that was pressed, and a press is a call every time rather than a state being toggled.
/// </summary>
public sealed class ViewerBrowserTests
{
    /// <summary>A viewer in front of a relay carrying one stream, with the seed behind it.</summary>
    private static (ViewerViewModel Viewer, SeededBackend Backend) Watching(string stream)
    {
        var backend = new SeededBackend("linux")
        {
            Relay = new RelayStatus { Reachable = true, Paths = { new RelayPath { Name = stream, Ready = true } } },
        };

        var session = new Session(backend, static action => action());
        session.Start();

        var viewer = Flows.Viewer(backend, session);
        viewer.Apply();

        return (viewer, backend);
    }

    /// <summary>
    /// The legs are the catalog's, in the order it named them.
    /// A shell that held its own list would be a shell deciding which protocols the relay serves a page for,
    /// which is the one thing the contract says it may not do (<c>docs/ipc-api.md</c>).
    /// </summary>
    [Fact]
    public void TheBrowserLegsAreTheOnesTheBackendNamed()
    {
        var (viewer, _) = Watching("bob");

        var row = Assert.Single(viewer.Streams);
        Assert.Equal(SeededBackend.BrowserLegs, row.BrowserLegs.Select(leg => leg.Value));
    }

    /// <summary>
    /// The player legs and the browser legs are two rosters and neither contains the other: no player opens
    /// WHEP, and a browser reaches neither SRT nor RTSP.
    /// One list serving both menus would have to be the narrower of them, which is a leg taken away from a
    /// reader that can run it.
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
    /// A press asks the backend for the leg it was pressed on, by the pair every viewer method takes.
    /// The stream and the leg together are the identity, because the relay re-serves one stream on all its
    /// listeners.
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
    /// A second press asks again, and that is the behaviour rather than a defect.
    /// A tab belongs to the browser that opened it: this side cannot read whether it is still there, so there
    /// is no state for a second press to find already true and nothing it could toggle off.
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
    /// Opening a page moves nothing on the viewer roster.
    /// It is what separates this from a watch: <c>StartWatch</c> launches a viewer the backend supervises and
    /// reports, and this hands an address to a program neither side hears from again.
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
    /// Two passes over an unchanged relay leave the same rows, commands and all.
    /// The legs are records so they compare equal, and the command behind one is made once by the row that
    /// owns it - a command rebuilt per pass would be a menu that loses a press to a poll.
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
