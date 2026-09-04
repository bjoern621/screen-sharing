using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What the empty grid says, one sentence per cause: outside the group, a group with nobody sharing,
/// or streams waiting unwatched. In Discord mode the way in is Discord's, so the sentence follows.
/// Quiet while a tile is up, while the first read is out, and while the rail's notice already speaks.
/// </summary>
public sealed class GridEmptyTests
{
    private static MembersState Group(bool joined) => new() { Joined = joined };

    private static string For(
        MembersState? members, int streams, int tiles, bool relayReady,
        DiscordState? discord = null, bool discordMode = false)
        => GridEmpty.For(members, discord, discordMode, streams, tiles, relayReady);

    [Fact]
    public void OutsideTheGroupNamesTheWayIn()
    {
        Assert.Equal(Cards.GridOutside, For(Group(joined: false), streams: 0, tiles: 0, relayReady: true));
    }

    [Fact]
    public void InDiscordModeAnUnlinkedMachineNamesTheLink()
    {
        Assert.Equal(
            Cards.GridUnlinked,
            For(Group(joined: false), streams: 0, tiles: 0, relayReady: true, discordMode: true));
    }

    [Fact]
    public void InDiscordModeALinkedMachineNamesTheChannel()
    {
        Assert.Equal(
            Cards.GridNoChannel,
            For(
                Group(joined: false), streams: 0, tiles: 0, relayReady: true,
                discord: new DiscordState { Linked = true }, discordMode: true));
    }

    [Fact]
    public void AJoinedGroupWithNothingLiveSaysSo()
    {
        Assert.Equal(Cards.GridIdle, For(Group(joined: true), streams: 0, tiles: 0, relayReady: true));
    }

    [Fact]
    public void WaitingStreamsPointAtTheList()
    {
        Assert.Equal(Cards.GridUnwatched, For(Group(joined: true), streams: 2, tiles: 0, relayReady: true));
    }

    [Fact]
    public void ATileOnScreenSilencesTheLine()
    {
        Assert.Equal("", For(Group(joined: true), streams: 2, tiles: 1, relayReady: true));
    }

    [Fact]
    public void AnUnansweredReadSaysNothing()
    {
        Assert.Equal("", For(members: null, streams: 0, tiles: 0, relayReady: true));
    }

    /// <summary>An unreachable relay is the rail notice's story, and the grid does not tell it twice.</summary>
    [Fact]
    public void AnUnreadyRelaySaysNothing()
    {
        Assert.Equal("", For(Group(joined: true), streams: 0, tiles: 0, relayReady: false));
    }
}
