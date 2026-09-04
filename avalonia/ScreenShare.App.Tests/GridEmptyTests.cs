using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What the empty grid says, one sentence per cause: outside the group, a group with nobody sharing,
/// or streams waiting unwatched.
/// Quiet while a tile is up, while the first read is out, and while the rail's notice already speaks.
/// </summary>
public sealed class GridEmptyTests
{
    private static MembersState Group(bool joined) => new() { Joined = joined };

    [Fact]
    public void OutsideTheGroupNamesTheWayIn()
    {
        Assert.Equal(Cards.GridOutside, GridEmpty.For(Group(joined: false), streams: 0, tiles: 0, relayReady: true));
    }

    [Fact]
    public void AJoinedGroupWithNothingLiveSaysSo()
    {
        Assert.Equal(Cards.GridIdle, GridEmpty.For(Group(joined: true), streams: 0, tiles: 0, relayReady: true));
    }

    [Fact]
    public void WaitingStreamsPointAtTheList()
    {
        Assert.Equal(Cards.GridUnwatched, GridEmpty.For(Group(joined: true), streams: 2, tiles: 0, relayReady: true));
    }

    [Fact]
    public void ATileOnScreenSilencesTheLine()
    {
        Assert.Equal("", GridEmpty.For(Group(joined: true), streams: 2, tiles: 1, relayReady: true));
    }

    [Fact]
    public void AnUnansweredReadSaysNothing()
    {
        Assert.Equal("", GridEmpty.For(members: null, streams: 0, tiles: 0, relayReady: true));
    }

    /// <summary>An unreachable relay is the rail notice's story, and the grid does not tell it twice.</summary>
    [Fact]
    public void AnUnreadyRelaySaysNothing()
    {
        Assert.Equal("", GridEmpty.For(Group(joined: true), streams: 0, tiles: 0, relayReady: false));
    }
}
