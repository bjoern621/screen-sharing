using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Members.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What the member list says in place of rows, one sentence per cause: the first read still out,
/// outside a group, or a group nobody else is in.
/// In Discord mode the way in is Discord's, so the sentence follows.
/// </summary>
public sealed class MembersEmptyTests
{
    private static MembersState Group(bool joined) => new() { Joined = joined };

    private static string For(
        MembersState? members, int rows = 0,
        DiscordState? discord = null, bool discordMode = false)
        => MembersEmpty.For(members, discord, discordMode, rows);

    [Fact]
    public void BeforeTheFirstReadTheCardSaysItIsReading()
    {
        Assert.Equal(Cards.MembersUnread, For(members: null));
    }

    [Fact]
    public void OutsideTheGroupNamesTheKeyAndTheName()
    {
        Assert.Equal(Cards.MembersOutside, For(Group(joined: false)));
    }

    [Fact]
    public void InDiscordModeAnUnlinkedMachineNamesTheLink()
    {
        Assert.Equal(Cards.MembersUnlinked, For(Group(joined: false), discordMode: true));
    }

    [Fact]
    public void InDiscordModeALinkedMachineNamesTheChannel()
    {
        Assert.Equal(
            Cards.MembersNoChannel,
            For(Group(joined: false), discord: new DiscordState { Linked = true }, discordMode: true));
    }

    [Fact]
    public void AJoinedGroupWithNobodyInItSaysSo()
    {
        Assert.Equal(Cards.MembersNone, For(Group(joined: true)));
    }

    [Fact]
    public void RowsOnScreenSilenceTheSentence()
    {
        Assert.Equal("", For(Group(joined: true), rows: 2));
        Assert.Equal("", For(Group(joined: false), rows: 2, discordMode: true));
    }
}
