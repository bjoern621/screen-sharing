using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Insights.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Tray.Model;
using ScreenShare.App.Features.Tray.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Tray menu, derived from the same commands the window presses.
/// What is on the air decides the one commit row: a start where nothing runs, a stop where something does.
/// A preset picked from the tray writes the draft the window holds, and restarts the stream where one is live.
/// Quit closes the window's decodes, then stops the stream, the latter only for a backend this shell started.
/// </summary>
public sealed class TrayTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>
    /// Window's state behind a tray: one session, one draft, both destinations, the tray reading them.
    /// Built as the shell builds them, fixtures answering from memory and the dispatcher running inline.
    /// </summary>
    private sealed record Fixture(
        PublishingBackend Backend,
        Session Session,
        FormSession Form,
        SetupViewModel Setup,
        InsightsViewModel Insights,
        TrayViewModel Tray)
    {
        /// <summary>Re-reads the running state and renders every reader, as the shell's pass would.</summary>
        public void Reload()
        {
            Session.Start();
            Session.Stop();
            Setup.Apply();
            Insights.Apply();
            Tray.Apply();
        }
    }

    private static Fixture Open(
        PublishingBackend backend, Func<bool>? owns = null, Func<CancellationToken, Task>? part = null)
    {
        var session = new Session(backend, Inline);
        var form = new FormSession(backend, session, Inline);
        var setup = new SetupViewModel(backend, form, session, Inline);
        var insights = new InsightsViewModel(backend, form, session, Inline);
        var tray = new TrayViewModel(
            backend, session, setup, insights,
            owns ?? (static () => true), part ?? (static _ => Task.CompletedTask), Inline);

        var opened = new Fixture(backend, session, form, setup, insights, tray);
        opened.Reload();
        return opened;
    }

    private static PublishState Live(string name) => new()
    {
        Live = new PublishState.Types.Live { Publish = new PublishSettings(), StreamName = name },
    };

    /// <summary>
    /// Keeps the draft with one field moved and puts it on the card, as a reader saving a preset would.
    /// Off the draft, so the repair returns the preset itself rather than a walked-to-legal version of it.
    /// </summary>
    private static async Task KeepAsync(Fixture opened, string name, int fps)
    {
        await opened.Form.Settled;

        var kept = opened.Form.Draft!.Publish.Clone();
        kept.Fps = fps;
        await opened.Backend.SavePresetAsync(name, kept);

        opened.Setup.Rail.Presets.RereadCommand.Execute(null);
        await opened.Setup.Rail.Presets.Settled;
        opened.Tray.Apply();
    }

    [Fact]
    public void NothingOnTheAirOffersAStart()
    {
        var opened = Open(new PublishingBackend());

        Assert.False(opened.Tray.Menu.IsLive);
        Assert.Equal(CommitCopy.Of(PublishCommit.Start).Label, opened.Tray.Menu.CommitLabel);
        Assert.True(opened.Tray.Menu.CanCommit);
    }

    [Fact]
    public void ALiveStreamOffersAStop()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        Assert.True(opened.Tray.Menu.IsLive);
        Assert.Equal(InsightsViewModel.StopLabel, opened.Tray.Menu.CommitLabel);
        Assert.True(opened.Tray.Menu.CanCommit);
    }

    /// <summary>The tray reads the same gate the review's button does, so the two cannot disagree.</summary>
    [Fact]
    public void AnUnreachableRelayLocksTheStart()
    {
        var opened = Open(new PublishingBackend
        {
            Relay = new RelayStatus { Reachable = false, Error = "no relay" },
        });

        Assert.False(opened.Tray.Menu.CanCommit);
    }

    [Fact]
    public void CommittingWhileNothingRunsStartsThePublish()
    {
        var opened = Open(new PublishingBackend());

        opened.Tray.Commit();

        Assert.Single(opened.Backend.Started);
        Assert.Empty(opened.Backend.Applied);
        Assert.Equal(0, opened.Backend.Stopped);
    }

    [Fact]
    public void CommittingWhileAStreamIsLiveStopsIt()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        opened.Tray.Commit();

        Assert.Equal(1, opened.Backend.Stopped);
        Assert.Empty(opened.Backend.Started);
        Assert.Empty(opened.Backend.Applied);
    }

    /// <summary>Built-in rows first, saved rows after them, both as the rail's card lists them.</summary>
    [Fact]
    public async Task TheMenuListsBuiltinThenSavedPresets()
    {
        var opened = Open(new PublishingBackend());
        await KeepAsync(opened, "work", fps: 120);

        var card = opened.Setup.Rail.Presets;
        var entries = opened.Tray.Menu.Presets;

        Assert.Equal(card.Builtin.Count + card.Rows.Count, entries.Count);
        Assert.Equal(
            card.Builtin.Select(row => row.Name),
            entries.Take(card.Builtin.Count).Select(entry => entry.Name));

        var saved = Assert.Single(entries, entry => entry.Kind == TrayPresetKind.Saved);
        Assert.Equal("work", saved.Name);
        Assert.True(saved.IsReachable);
    }

    /// <summary>One press is on the air on the preset, the same semantics as the strip menu's rows.</summary>
    [Fact]
    public async Task PickingAPresetWhileNothingRunsStartsSharingOnIt()
    {
        var opened = Open(new PublishingBackend());
        await KeepAsync(opened, "work", fps: 120);

        var entry = opened.Tray.Menu.Presets.Single(entry => entry.Kind == TrayPresetKind.Saved);
        opened.Tray.UsePreset(entry);
        await opened.Form.Settled;

        Assert.Equal(120, opened.Form.Draft!.Publish.Fps);
        var started = Assert.Single(opened.Backend.Started);
        Assert.Equal(120, started.Publish.Fps);
        Assert.Empty(opened.Backend.Applied);
    }

    /// <summary>A switch while live is a restart on the preset, the same press the review's apply is.</summary>
    [Fact]
    public async Task PickingAPresetWhileLiveRestartsTheStreamOnIt()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });
        await KeepAsync(opened, "work", fps: 120);

        var entry = opened.Tray.Menu.Presets.Single(entry => entry.Kind == TrayPresetKind.Saved);
        opened.Tray.UsePreset(entry);

        var applied = Assert.Single(opened.Backend.Applied);
        Assert.Equal(120, applied.Publish.Fps);
        Assert.Empty(opened.Backend.Started);
    }

    [Fact]
    public async Task ThePickedPresetIsMarkedAsCurrent()
    {
        var opened = Open(new PublishingBackend());
        await KeepAsync(opened, "work", fps: 120);

        opened.Tray.UsePreset(opened.Tray.Menu.Presets.Single(entry => entry.Kind == TrayPresetKind.Saved));
        await opened.Form.Settled;
        opened.Tray.Apply();

        var saved = opened.Tray.Menu.Presets.Single(entry => entry.Kind == TrayPresetKind.Saved);
        Assert.True(saved.IsCurrent);
    }

    /// <summary>A row missing from the card, or one nothing here reaches, does nothing.</summary>
    [Fact]
    public void AnUnknownPresetSwitchesNothing()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        opened.Tray.UsePreset(new TrayPresetEntry
        {
            Kind = TrayPresetKind.Builtin,
            Id = "nope",
            Name = "nope",
            IsCurrent = false,
            IsReachable = false,
        });

        Assert.Empty(opened.Backend.Started);
        Assert.Empty(opened.Backend.Applied);
    }

    [Fact]
    public void QuitStopsTheStreamForABackendThisShellStarted()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        var quits = 0;
        opened.Tray.QuitRequested += () => quits++;

        opened.Tray.QuitCommand.Execute(null);

        Assert.Equal(1, opened.Backend.Stopped);
        Assert.Equal(1, quits);
    }

    /// <summary>A backend this shell did not start keeps publishing, so its stream is not stopped either.</summary>
    [Fact]
    public void QuitLeavesTheStreamOfABackendItDidNotStart()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") }, owns: static () => false);

        var quits = 0;
        opened.Tray.QuitRequested += () => quits++;

        opened.Tray.QuitCommand.Execute(null);

        Assert.Equal(0, opened.Backend.Stopped);
        Assert.Equal(1, quits);
    }

    [Fact]
    public void QuitWithNothingOnTheAirStopsNothing()
    {
        var opened = Open(new PublishingBackend());

        var quits = 0;
        opened.Tray.QuitRequested += () => quits++;

        opened.Tray.QuitCommand.Execute(null);

        Assert.Equal(0, opened.Backend.Stopped);
        Assert.Equal(1, quits);
    }

    /// <summary>
    /// The window's decodes are the shell's alone, whichever backend runs them,
    /// so they close on every quit and before the shutdown is asked for.
    /// </summary>
    [Fact]
    public void QuitPartsTheWindowBeforeAskingToShutDown()
    {
        var order = new List<string>();
        var opened = Open(
            new PublishingBackend { Publish = Live("lab04") },
            owns: static () => false,
            part: _ =>
            {
                order.Add("part");
                return Task.CompletedTask;
            });
        opened.Tray.QuitRequested += () => order.Add("quit");

        opened.Tray.QuitCommand.Execute(null);

        Assert.Equal(["part", "quit"], order);
        Assert.Equal(0, opened.Backend.Stopped);
    }

    /// <summary>A part that never answers is waited out, the exit being what was asked for.</summary>
    [Fact]
    public async Task QuitOutwaitsAPartThatNeverAnswers()
    {
        var quit = new TaskCompletionSource();
        var opened = Open(
            new PublishingBackend { Publish = Live("lab04") },
            part: async cancellation => await Task.Delay(Timeout.Infinite, cancellation));
        opened.Tray.QuitRequested += () => quit.SetResult();

        opened.Tray.QuitCommand.Execute(null);

        await quit.Task.WaitAsync(TimeSpan.FromSeconds(10));
        Assert.Equal(1, opened.Backend.Stopped);
    }

    /// <summary>The pass runs on every shell render, so an unchanged menu has to notify nothing.</summary>
    [Fact]
    public void ASecondRenderPassOverAnUnchangedStateNotifiesNothing()
    {
        var opened = Open(new PublishingBackend { Publish = Live("lab04") });

        var moved = new List<string?>();
        opened.Tray.PropertyChanged += (_, e) => moved.Add(e.PropertyName);

        opened.Tray.Apply();
        opened.Tray.Apply();

        Assert.Empty(moved);
    }
}
