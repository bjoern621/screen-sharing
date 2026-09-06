using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Insights.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Shell.Go.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Strip commit, derived from the same commands the window presses.
/// The button carries the review's own commit, so the label, the guard and the wait stay one each.
/// A preset picked from the strip writes the draft and commits it, live or not.
/// The summary line repeats the publish groups' shorthands.
/// </summary>
public sealed class GoStripTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>
    /// Window's state behind the strip: one session, one draft, both destinations, the strip reading them.
    /// Built as the shell builds them, fixtures answering from memory and the dispatcher running inline.
    /// </summary>
    private sealed record Fixture(
        PublishingBackend Backend,
        Session Session,
        FormSession Form,
        SetupViewModel Setup,
        InsightsViewModel Insights,
        GoViewModel Go)
    {
        /// <summary>Re-reads the running state and renders every reader, as the shell's pass would.</summary>
        public void Reload()
        {
            Session.Start();
            Session.Stop();
            Setup.Apply();
            Insights.Apply();
            Go.Apply();
        }
    }

    private static Fixture Open(PublishingBackend backend)
    {
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);
        var insights = new InsightsViewModel(backend, form, session, Inline);
        var go = new GoViewModel(session, form, setup, insights);

        var opened = new Fixture(backend, session, form, setup, insights, go);
        opened.Reload();
        return opened;
    }

    private static PublishState Live(string name) => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = name },
    };

    /// <summary>Keeps the draft with one field moved and puts it on the card, as a reader saving a preset would.</summary>
    private static async Task KeepAsync(Fixture opened, string name, int fps)
    {
        await opened.Form.Settled;

        var kept = opened.Form.Draft!.Publish.Clone();
        kept.Fps = fps;
        await opened.Backend.SavePresetAsync(name, kept);

        opened.Setup.Rail.Presets.RereadCommand.Execute(null);
        await opened.Setup.Rail.Presets.Settled;
        opened.Go.Apply();
    }

    [Fact]
    public void NothingOnTheAirOffersAStart()
    {
        var opened = Open(new PublishingBackend());

        Assert.False(opened.Go.IsLive);
        Assert.Equal(CommitCopy.Of(PublishCommit.Start).Label, opened.Go.CommitLabel);
        Assert.True(opened.Go.CommitCommand.CanExecute(null));
    }

    [Fact]
    public void TheStripPressStartsThePublish()
    {
        var opened = Open(new PublishingBackend());

        opened.Go.CommitCommand.Execute(null);

        Assert.Single(opened.Backend.Started);
        Assert.Empty(opened.Backend.Applied);
    }

    /// <summary>The strip reads the same gate the review's button does, so the two cannot disagree.</summary>
    [Fact]
    public void AnUnreachableRelayLocksTheCommitAndNamesWhy()
    {
        var opened = Open(new PublishingBackend
        {
            Relay = new RelayStatus { Reachable = false, Error = "no relay" },
        });

        Assert.False(opened.Go.CommitCommand.CanExecute(null));
        Assert.Equal("no relay", opened.Go.Blocked);
    }

    /// <summary>
    /// The menu's commit row is marked by what pressing it does, off the row its word comes from.
    /// The two marks differ, one that cannot tell a start from a restart saying nothing.
    /// </summary>
    [Fact]
    public void TheCommitMarkFollowsWhatPressingItWillDo()
    {
        var idle = Open(new PublishingBackend());
        var live = Open(new PublishingBackend { Publish = Live("lab04") });

        Assert.Equal(CommitCopy.Of(PublishCommit.Start).Glyph, idle.Go.CommitGlyph);
        Assert.Equal(CommitCopy.Of(PublishCommit.Apply).Glyph, live.Go.CommitGlyph);
        Assert.NotEqual(CommitCopy.Of(PublishCommit.Start).Glyph, CommitCopy.Of(PublishCommit.Apply).Glyph);
    }

    /// <summary>A stream on the air relabels the press rather than blocking it.</summary>
    [Fact]
    public void ALiveStreamRelabelsTheCommitToApply()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        Assert.True(opened.Go.IsLive);
        Assert.Equal(CommitCopy.Of(PublishCommit.Apply).Label, opened.Go.CommitLabel);
    }

    /// <summary>One press on a preset goes live on it, where the tray's pick only writes the draft.</summary>
    [Fact]
    public async Task PickingASavedPresetStartsSharingOnIt()
    {
        var opened = Open(new PublishingBackend());
        await KeepAsync(opened, "work", fps: 120);

        opened.Go.UseSaved("work");
        await opened.Form.Settled;

        Assert.Equal(120, opened.Form.Draft!.Publish.Fps);
        var started = Assert.Single(opened.Backend.Started);
        Assert.Equal(120, started.Publish.Fps);
        Assert.Empty(opened.Backend.Applied);
    }

    [Fact]
    public async Task PickingABuiltinWhileLiveRestartsOnIt()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });
        await opened.Form.Settled;
        opened.Setup.Apply();

        var row = opened.Setup.Rail.Presets.Builtin.First(row => row.IsReachable);
        opened.Go.UseBuiltin(row.Key);

        Assert.Single(opened.Backend.Applied);
        Assert.Empty(opened.Backend.Started);
    }

    /// <summary>A row missing from the card, or one nothing here reaches, does nothing.</summary>
    [Fact]
    public void AnUnknownPresetCommitsNothing()
    {
        var opened = Open(new PublishingBackend());

        opened.Go.UseBuiltin("nope");
        opened.Go.UseSaved("nope");

        Assert.Empty(opened.Backend.Started);
        Assert.Empty(opened.Backend.Applied);
    }

    /// <summary>The line repeats the publish groups' own shorthands, so it says nothing the form does not.</summary>
    [Fact]
    public async Task TheSummaryRepeatsThePublishGroupShorthands()
    {
        var opened = Open(new PublishingBackend());
        await opened.Form.Settled;
        opened.Setup.Apply();
        opened.Go.Apply();

        var form = opened.Form.Form!;
        var parts = new[] { "source", "quality", "audio", "transport" }
            .Select(key => opened.Session.Words.Shorthand(
                form.Groups.FirstOrDefault(group => group.Key == key), form.Settings))
            .Where(part => part.Length > 0);

        var expected = string.Join(" · ", parts);
        Assert.NotEqual("", expected);
        Assert.Equal(expected, opened.Go.Summary);
    }

    /// <summary>
    /// A stream built from the draft has nothing to apply, so the press is the way out instead.
    /// The one gesture that is always there while sharing, from whichever destination is showing.
    /// </summary>
    [Fact]
    public async Task AStreamRunningTheDraftTurnsThePressIntoAStop()
    {
        var backend = new PublishingBackend();
        backend.Publish = backend.Running();
        var opened = Open(backend);
        await opened.Form.Settled;
        opened.Setup.Apply();

        Assert.True(opened.Go.ShowsStop);
        Assert.Equal(InsightsViewModel.StopLabel, opened.Go.PressLabel);
        Assert.Same(opened.Go.StopCommand, opened.Go.Press);
        Assert.True(opened.Go.Press.CanExecute(null));
    }

    /// <summary>Read off the draft on every pass, so a value moved hands the press back to the apply.</summary>
    [Fact]
    public async Task AValueMovedWhileLiveHandsThePressBackToTheApply()
    {
        var backend = new PublishingBackend();
        backend.Publish = backend.Running();
        var opened = Open(backend);
        await opened.Form.Settled;
        opened.Setup.Apply();
        var fps = opened.Form.Draft!.Publish.Fps;

        opened.Form.Write("publish.fps", new FieldValue { Number = 120 });

        Assert.False(opened.Go.ShowsStop);
        Assert.Equal(CommitCopy.Of(PublishCommit.Apply).Label, opened.Go.PressLabel);
        Assert.Same(opened.Go.CommitCommand, opened.Go.Press);

        opened.Form.Write("publish.fps", new FieldValue { Number = fps });

        Assert.True(opened.Go.ShowsStop);
    }

    /// <summary>The press ends the stream through the screen's own stop, so one command answers for both.</summary>
    [Fact]
    public async Task TheStripPressEndsTheStreamItRuns()
    {
        var backend = new PublishingBackend();
        backend.Publish = backend.Running();
        var opened = Open(backend);
        await opened.Form.Settled;
        opened.Setup.Apply();

        opened.Go.Press.Execute(null);

        Assert.Equal(1, backend.Stopped);
        Assert.Empty(backend.Applied);
        Assert.Empty(backend.Started);
    }

    /// <summary>The pass runs on every shell render, so an unchanged strip has to notify nothing.</summary>
    [Fact]
    public void ASecondRenderPassOverAnUnchangedStateNotifiesNothing()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        var moved = new List<string?>();
        opened.Go.PropertyChanged += (_, e) => moved.Add(e.PropertyName);

        opened.Go.Apply();
        opened.Go.Apply();

        Assert.Empty(moved);
    }
}
