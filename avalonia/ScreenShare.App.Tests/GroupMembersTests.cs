using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Members.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The group is a reading and never a list this shell keeps:
/// every member's own app states its presence, and one that stopped drops out by not appearing.
/// The card renders whatever the last answer named, says which row is this machine,
/// and presses nothing: the group key and the name in the settings are what put this machine in a group.
/// </summary>
public sealed class GroupMembersTests
{
    private static MembersViewModel Card() => new();

    private static Member Member(string name, bool self = false) => new()
    {
        MemberId = $"id-{name}",
        DisplayName = name,
        Self = self,
    };

    private static MembersState Group(bool joined, params Member[] members)
    {
        var state = new MembersState { Joined = joined };
        state.Members.AddRange(members);
        return state;
    }

    [Fact]
    public void ARowPerMemberSaysWhichIsThisMachine()
    {
        var card = Card();

        card.Reported = Group(joined: true, Member("Björn", self: true), Member("Ada"));
        card.Apply();

        Assert.Equal(2, card.Rows.Count);
        Assert.Equal("Björn", card.Rows[0].Name);
        Assert.True(card.Rows[0].IsSelf);
        Assert.Equal(Cards.MemberSelf, card.Rows[0].Detail);
        Assert.Equal("Ada", card.Rows[1].Name);
        Assert.False(card.Rows[1].IsSelf);
        Assert.False(card.Rows[1].HasDetail);
    }

    /// <summary>
    /// A member the service named with no display name is still a member,
    /// and the id is what a bug report carries.
    /// </summary>
    [Fact]
    public void AMemberWithNoNameIsListedUnderItsIdentity()
    {
        var card = Card();

        card.Reported = Group(joined: true, new Member { MemberId = "TWFOWD4E7QSXK4YQ" });
        card.Apply();

        Assert.Equal("TWFOWD4E7QSXK4YQ", Assert.Single(card.Rows).Name);
    }

    /// <summary>
    /// A name another member holds leaves the list empty on a group that has people in it,
    /// so the refusal is the whole of what the reader can act on.
    /// </summary>
    [Fact]
    public void ARefusedNameIsSaidBesideTheList()
    {
        var card = Card();

        var state = Group(joined: false);
        state.Refusal = new Text { Code = TextCode.GroupNameTaken };
        card.Reported = state;
        card.Apply();

        Assert.True(card.HasRefusal);
        Assert.Contains("holds that name", card.Refusal);
    }

    [Fact]
    public void AGroupKeyWithNoNameBesideItSaysWhatIsMissing()
    {
        var card = Card();

        var state = Group(joined: false);
        state.Refusal = new Text { Code = TextCode.GroupNameMissing };
        card.Reported = state;
        card.Apply();

        Assert.Contains("needs a name for this computer", card.Refusal);
    }

    /// <summary>Three absences, and a reader has a different thing to do next in each.</summary>
    [Fact]
    public void AnUnreadGroupAnUnsetOneAndAnEmptyOneReadDifferently()
    {
        var card = Card();

        card.Apply();
        var unread = card.Notice;

        card.Reported = Group(joined: false);
        card.Apply();
        var outside = card.Notice;

        card.Reported = Group(joined: true);
        card.Apply();
        var empty = card.Notice;

        Assert.False(card.HasRows);
        Assert.NotEqual(unread, outside);
        Assert.NotEqual(outside, empty);
        Assert.All([unread, outside, empty], notice => Assert.NotEqual("", notice));
    }

    /// <summary>
    /// Rows are records, so an unchanged answer leaves the bound list where it is,
    /// and nothing under the pointer repaints.
    /// </summary>
    [Fact]
    public void ASecondPassOverOneAnswerRebuildsNothing()
    {
        var card = Card();

        card.Reported = Group(joined: true, Member("Björn", self: true), Member("Ada"));
        card.Apply();
        var first = card.Rows[0];

        card.Apply();

        Assert.Same(first, card.Rows[0]);
    }
}
