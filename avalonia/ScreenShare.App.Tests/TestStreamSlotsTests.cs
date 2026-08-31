using ScreenShare.Api.V1;
using ScreenShare.App.Features.Broadcast.TestStreams.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// A count says how many synthetic publishers are up and nothing about which,
/// so a slot whose child died is visible only as a row of its own:
/// which slot it is, which attempt it is on, why it carries no publisher,
/// and the two strings a reader takes to a bug report.
/// </summary>
public sealed class TestStreamSlotsTests
{
    private static TestStreamState Set(int running, params TestStreamSlot[] slots)
    {
        var state = new TestStreamState { RunningCount = running };
        state.Slots.AddRange(slots);
        return state;
    }

    private static TestStreamSlot Running(int slot, string name) => new()
    {
        Slot = slot,
        Name = name,
        Running = true,
        Attempt = 1,
    };

    [Fact]
    public void ASlotCarryingNoPublisherSaysWhichAttemptAndWhy()
    {
        var card = new TestStreamsViewModel
        {
            Reported = Set(1,
                Running(0, "test-0-bars"),
                new TestStreamSlot
                {
                    Slot = 1,
                    Name = "test-1-ball",
                    Running = false,
                    Attempt = 4,
                    Cause = new Text { Code = TextCode.StreamLeftTheRelay },
                    Message = "srt: connection was rejected",
                    LogPath = "/tmp/screenshare/test-1-ball.log",
                }),
        };

        Assert.Equal(2, card.Rows.Count);

        var down = card.Rows[1];
        Assert.False(down.IsRunning);
        Assert.Contains("test-1-ball", down.Label);
        Assert.Contains("4", down.Attempt);
        Assert.Contains("stopped arriving", down.Cause);
        Assert.Equal("srt: connection was rejected", down.Message);
        Assert.Equal("/tmp/screenshare/test-1-ball.log", down.LogPath);
    }

    /// <summary>A running slot has nothing to answer for, so it carries no cause and no last words.</summary>
    [Fact]
    public void ARunningSlotCarriesNoReason()
    {
        var card = new TestStreamsViewModel { Reported = Set(1, Running(0, "test-0-bars")) };

        var row = Assert.Single(card.Rows);
        Assert.True(row.IsRunning);
        Assert.Equal("", row.Cause);
        Assert.Equal("", row.Message);
    }

    [Fact]
    public void TheHeadingCountsWhatIsSendingAgainstWhatTheSetHolds()
    {
        var card = new TestStreamsViewModel
        {
            Reported = Set(1,
                Running(0, "test-0-bars"),
                new TestStreamSlot { Slot = 1, Name = "test-1-ball", Running = false, Attempt = 2 }),
        };

        Assert.Contains("1", card.Summary);
        Assert.Contains("2", card.Summary);
    }

    [Fact]
    public void ASetWithNoSlotsSaysSoRatherThanDrawingAnEmptyList()
    {
        var card = new TestStreamsViewModel { Reported = Set(0) };

        Assert.False(card.HasRows);
        Assert.NotEqual("", card.Notice);
    }

    /// <summary>An unread state and an empty set are different things, and a reader acts on them differently.</summary>
    [Fact]
    public void AnUnreadSetReadsDifferentlyFromAnEmptyOne()
    {
        var unread = new TestStreamsViewModel();
        var empty = new TestStreamsViewModel { Reported = Set(0) };

        Assert.NotEqual(unread.Notice, empty.Notice);
        Assert.NotEqual("", unread.Notice);
    }

    [Fact]
    public void ASecondPassOverOneAnswerRebuildsNothing()
    {
        var card = new TestStreamsViewModel { Reported = Set(1, Running(0, "test-0-bars")) };
        var first = card.Rows[0];

        card.Apply();

        Assert.Same(first, card.Rows[0]);
    }
}
