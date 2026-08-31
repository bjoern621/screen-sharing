using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Viewer.Members.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The group is a reading and never a list this shell keeps:
/// every member's own app states its presence, and one that stopped drops out by not appearing.
/// The card renders whatever the last answer named, says which row is this machine,
/// and offers exactly one of the two actions.
/// </summary>
public sealed class GroupMembersTests
{
    /// <summary>Inline, so a press has been answered by the time it returns.</summary>
    private static readonly Action<Action> Inline = action => action();

    private static MembersViewModel Card(IBackend backend) => new(backend, Inline);

    private static Member Member(string name, bool publishing = false, bool self = false) => new()
    {
        MemberId = $"id-{name}",
        DisplayName = name,
        Publishing = publishing,
        Self = self,
    };

    private static MembersState Group(bool joined, params Member[] members)
    {
        var state = new MembersState { Joined = joined };
        state.Members.AddRange(members);
        return state;
    }

    [Fact]
    public void ARowPerMemberSaysWhoIsSendingAndWhichIsThisMachine()
    {
        var card = Card(new SeededBackend("linux"));

        card.Reported = Group(joined: true, Member("Björn", publishing: true, self: true), Member("Ada"));
        card.Apply();

        Assert.Equal(2, card.Rows.Count);
        Assert.Equal("Björn", card.Rows[0].Name);
        Assert.True(card.Rows[0].IsPublishing);
        Assert.True(card.Rows[0].IsSelf);
        Assert.Equal("Ada", card.Rows[1].Name);
        Assert.False(card.Rows[1].IsPublishing);
        Assert.False(card.Rows[1].IsSelf);
    }

    /// <summary>
    /// A member the service named with no display name is still a member,
    /// and the id is what a bug report carries.
    /// </summary>
    [Fact]
    public void AMemberWithNoNameIsListedUnderItsIdentity()
    {
        var card = Card(new SeededBackend("linux"));

        card.Reported = Group(joined: true, new Member { MemberId = "TWFOWD4E7QSXK4YQ" });
        card.Apply();

        Assert.Equal("TWFOWD4E7QSXK4YQ", Assert.Single(card.Rows).Name);
    }

    [Fact]
    public void AMachineOutsideTheGroupIsOfferedJoinAndNotLeave()
    {
        var card = Card(new SeededBackend("linux"));

        card.Reported = Group(joined: false);
        card.Apply();

        Assert.True(card.CanJoin);
        Assert.False(card.CanLeave);
    }

    [Fact]
    public void AMachineInTheGroupIsOfferedLeaveAndNotJoin()
    {
        var card = Card(new SeededBackend("linux"));

        card.Reported = Group(joined: true, Member("Björn", self: true));
        card.Apply();

        Assert.False(card.CanJoin);
        Assert.True(card.CanLeave);
    }

    /// <summary>
    /// A name another member holds leaves the list empty on a group that has people in it,
    /// so the refusal is the whole of what the reader can act on.
    /// </summary>
    [Fact]
    public void ARefusedNameIsSaidBesideTheActions()
    {
        var card = Card(new SeededBackend("linux"));

        var state = Group(joined: false);
        state.Refusal = new Text { Code = TextCode.GroupNameTaken };
        card.Reported = state;
        card.Apply();

        Assert.True(card.HasRefusal);
        Assert.Contains("holds that name", card.Refusal);
    }

    [Fact]
    public void AGroupJoinedWithNoNameSetSaysWhatIsMissing()
    {
        var card = Card(new SeededBackend("linux"));

        var state = Group(joined: false);
        state.Refusal = new Text { Code = TextCode.GroupNameMissing };
        card.Reported = state;
        card.Apply();

        Assert.Contains("needs a name for this computer", card.Refusal);
    }

    /// <summary>Three absences, and a reader has a different thing to do next in each.</summary>
    [Fact]
    public void AnUnreadGroupAnUnjoinedOneAndAnEmptyOneReadDifferently()
    {
        var card = Card(new SeededBackend("linux"));

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

    [Fact]
    public void PressingJoinAsksTheBackendToJoin()
    {
        var backend = new SeededBackend("linux");
        var card = Card(backend);

        card.Reported = Group(joined: false);
        card.Apply();
        card.Join.Execute(null);

        Assert.Equal(1, backend.Joins);
        Assert.Equal(0, backend.Leaves);
    }

    [Fact]
    public void PressingLeaveAsksTheBackendToLeave()
    {
        var backend = new SeededBackend("linux");
        var card = Card(backend);

        card.Reported = Group(joined: true, Member("Björn", self: true));
        card.Apply();
        card.Leave.Execute(null);

        Assert.Equal(1, backend.Leaves);
        Assert.Equal(0, backend.Joins);
    }

    /// <summary>A refused press is the backend's own sentence, shown as it arrived.</summary>
    [Fact]
    public void ARefusedPressShowsWhatTheBackendSaid()
    {
        var backend = new SeededBackend("linux") { GroupRefusal = "that name is taken in this group" };
        var card = Card(backend);

        card.Reported = Group(joined: false);
        card.Apply();
        card.Join.Execute(null);

        Assert.True(card.HasRefusal);
        Assert.Equal("that name is taken in this group", card.Refusal);
    }

    /// <summary>
    /// Rows are records, so an unchanged answer leaves the bound list where it is,
    /// and nothing under the pointer repaints.
    /// </summary>
    [Fact]
    public void ASecondPassOverOneAnswerRebuildsNothing()
    {
        var card = Card(new SeededBackend("linux"));

        card.Reported = Group(joined: true, Member("Björn", self: true), Member("Ada"));
        card.Apply();
        var first = card.Rows[0];

        card.Apply();

        Assert.Same(first, card.Rows[0]);
    }
}
