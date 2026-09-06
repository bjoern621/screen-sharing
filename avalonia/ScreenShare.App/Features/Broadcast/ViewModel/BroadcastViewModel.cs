using Avalonia;
using Avalonia.Controls;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.ConfigCard.ViewModel;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Plots.ViewModel;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using ScreenShare.App.Features.Broadcast.SessionLog.ViewModel;
using ScreenShare.App.Features.Broadcast.TestStreams.ViewModel;
using ScreenShare.App.Features.Broadcast.ViewerTable.ViewModel;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.ViewModel;

/// <summary>
/// Broadcast screen: the live overview, and read-only about everything else.
///
/// Reachable whether or not a stream is running.
/// The session log outlives the stream it records, and why the last pipeline exited is what a publisher goes
/// looking for once it has, so every card here states its idle reading rather than being kept off screen until
/// there is a stream to describe.
///
/// A control appears in exactly one window, so this owns only actions safe while a stream runs and hands
/// configuration to a card that shows it and cannot write it.
/// The one way out is <see cref="BroadcastAction.EditInSetup"/>, which navigates rather than editing here.
///
/// Holds no reading of its own.
/// <see cref="Backend.Session"/> owns the running state and this reads it through on every pass, so a card cannot
/// go on describing a stream that has stopped (<c>docs/development-principles.md</c>, "State has one owner").
/// <see cref="Apply"/> is the one render function and pushes one composed reading into every card, so two cards
/// cannot disagree about the stream they describe.
///
/// Pause, force keyframe and reconnect name effects the control contract does not carry
/// (<c>docs/ipc-api.md</c>, "The rule").
/// They are drawn disabled and carrying why, the treatment the settings form gives a blocked concept: removing
/// them would hide that the capability is missing.
///
/// Stop is the one that is real, and pressing it writes nothing here: the reply carries no state and what
/// the stream became arrives on the event stream.
/// </summary>
public sealed class BroadcastViewModel : Observable
{
    /// <summary>
    /// Why the three header actions are inert.
    /// One sentence for all three, being one fact about the contract rather than three about the buttons.
    /// </summary>
    private const string UnbackedReason =
        "The control contract has no pause, keyframe or reconnect effect: it carries the reads "
        + "a screen draws from and the few effects a user asks for by name.";

    private readonly IBackend _backend;

    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// What each figure last measured, for the passes where nothing did.
    /// Filled into the reading before it is handed out, so the cards and the header hold one figure rather than
    /// one holding and another blinking.
    /// </summary>
    private readonly HeldFigures _held = new();

    /// <summary>
    /// Where a refusal the backend answered with is shown.
    /// Set from outside, so a screen built with no shell around it still renders and still presses buttons.
    /// </summary>
    private Action<string> _report = static _ => { };

    /// <summary>
    /// Settings the config card was last resolved against.
    /// A pipeline emits a sample a second and its settings do not move while it runs, so an unchanged pipeline
    /// is not re-resolved per event.
    /// </summary>
    private Settings? _described;

    /// <summary>What that resolve answered with, null until one has landed.</summary>
    private Form? _form;

    /// <summary>Cancels a resolve in flight when the running pipeline moves off it.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>
    /// Width the window last stated, infinite until it states one (<see cref="SetWindowWidth"/>).
    /// </summary>
    private double _window = double.PositiveInfinity;

    /// <summary>Raised once per press, never during a render.</summary>
    public event Action<BroadcastAction>? ActionRequested;

    /// <param name="form">
    /// Settings the backend is holding, handed to the preview and read nowhere else on this screen:
    /// its end-to-end route needs the leg a viewer receives on.
    /// A different thing from <see cref="_form"/>, what the running pipeline's settings resolved to.
    /// </param>
    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// An effect's answer and a resolve's both land on whichever thread the transport completed on, and every
    /// binding here tolerates one writer thread.
    /// </param>
    public BroadcastViewModel(IBackend backend, FormSession form, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a broadcast screen asks the backend to act");
        Assert.NotNull(form, "a broadcast screen describes the settings a stream is running on");
        Assert.NotNull(session, "a broadcast screen renders the session's running state");
        Assert.NotNull(dispatch, "a broadcast screen needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        Stats = new HeaderStatsViewModel();

        // The one card here that asks the backend for anything.
        // Its end-to-end route receives this machine's own stream back off the relay, so it takes the boundary,
        // the running state and the leg a viewer receives on rather than the composed reading alone.
        Preview = new PreviewViewModel(backend, form, session, dispatch);
        Config = new ConfigCardViewModel();
        Viewers = new ViewerTableViewModel();
        Plots = new PlotsViewModel();

        // Synthetic publishers belong on this screen because they are what this machine is putting on the relay,
        // which is what this screen is about.
        // The one publish above them is the real one.
        TestStreams = new TestStreamsViewModel();
        Log = new SessionLogViewModel(OpenLogAsync, dispatch);

        // Beside the figures where the window carries both, over them where it does not
        // (Shell/Model/SideColumns.cs).
        CardsColumn = new SideColumnViewModel(
            SideColumns.BroadcastCards,
            "Show the preview, the configuration and the test streams",
            "Hide the preview, the configuration and the test streams");

        // Constructed unpressable rather than disabled by a later pass, so no instant exists in which one
        // of the three works.
        PauseCommand = new DelegateCommand(() => Request(BroadcastAction.Pause), static () => false);
        ForceKeyframeCommand = new DelegateCommand(() => Request(BroadcastAction.ForceKeyframe), static () => false);
        ReconnectCommand = new DelegateCommand(() => Request(BroadcastAction.Reconnect), static () => false);

        // Ending a stream crosses to the backend and the encoder it brings down, so the button waits
        // on the answer and refuses a second press while the first is out.
        StopCommand = new PendingCommand(() => PerformAsync(_backend.StopPublishAsync), dispatch, () => IsLive);

        // The one thing a card raises as news.
        // Opening the log is not: a card that waits on a call has to be what knows the call is out.
        Config.EditRequested += () => Request(BroadcastAction.EditInSetup);

        Apply();
    }

    /// <summary>
    /// Hands the screen somewhere to put a refusal the backend answered with.
    /// Idempotent, and renders after, so a screen that already holds one shows it on attach.
    /// </summary>
    public void Attach(Action<string> report)
    {
        Assert.NotNull(report, "a refusal needs a window to report it in");

        _report = report;
        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;
    private string _refusal = "";
    private bool _hasRefusal;

    public HeaderStatsViewModel Stats { get; }

    public PreviewViewModel Preview { get; }

    public ConfigCardViewModel Config { get; }

    public ViewerTableViewModel Viewers { get; }

    public PlotsViewModel Plots { get; }

    /// <summary>
    /// Where the preview, the configuration and the test streams stand: beside the live figures,
    /// or over them on a window with the width for one column (<c>docs/design-language.md</c>, "Narrow windows").
    /// The figures keep the body, a stream's readings being what this screen is opened for.
    /// </summary>
    public SideColumnViewModel CardsColumn { get; }

    /// <summary>
    /// Which edge of the header band the live actions take.
    /// Beside the figures where the window carries both, under them where it does not: actions that took the row
    /// would leave the figures a column one figure wide.
    /// </summary>
    public Dock ActionsDock => SideColumns.BroadcastActions.FitsBeside(_window) ? Dock.Right : Dock.Bottom;

    /// <summary>Gap over the actions, which they need only while they stand under the figures.</summary>
    public Thickness ActionsInset => ActionsDock == Dock.Bottom ? new Thickness(0, 10, 0, 0) : default;

    /// <summary>
    /// States the width the window has, which decides where the cards and the actions stand.
    /// Idempotent: the same width twice moves nothing.
    /// </summary>
    public void SetWindowWidth(double width)
    {
        Assert.That(width > 0, "a window states a width it has", width);

        _window = width;
        CardsColumn.SetWindowWidth(width);
        OnPropertyChanged(nameof(ActionsDock));
        OnPropertyChanged(nameof(ActionsInset));
    }

    /// <summary>States whether the window is on screen, which the preview's picture follows. Idempotent.</summary>
    public void SetWindowShown(bool shown) => Preview.SetWindowShown(shown);

    /// <summary>
    /// Synthetic publishers this machine runs, a row per slot.
    /// The count says how many are up and nothing about which, so a slot waiting out a relaunch is readable
    /// from its own row alone.
    /// </summary>
    public TestStreamsViewModel TestStreams { get; }

    public SessionLogViewModel Log { get; }

    public DelegateCommand PauseCommand { get; }

    public DelegateCommand ForceKeyframeCommand { get; }

    public DelegateCommand ReconnectCommand { get; }

    /// <summary>
    /// Word on the stop control, wherever it is drawn: this screen's button and the tray's row.
    /// One place, so the two cannot drift.
    /// </summary>
    public const string StopLabel = "Stop sharing";

    /// <summary>The one control on this screen ending the stream.</summary>
    public PendingCommand StopCommand { get; }

    /// <summary>
    /// Reading every card on this screen describes, composed from the session's whole states on each pass.
    /// An output rather than an input: the backend alone knows it, so nothing writes it from outside.
    /// </summary>
    public BroadcastSnapshot Snapshot { get => _snapshot; private set => Set(ref _snapshot, value); }

    /// <summary>Sentence the inert actions' tooltip carries.</summary>
    public string UnbackedActions => UnbackedReason;

    /// <summary>Backend's own sentence for something this screen asked and was refused, empty otherwise.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    /// <summary>
    /// Read through rather than mirrored, so a command asked twice in one pass cannot answer from a stale copy
    /// of the reading.
    /// </summary>
    private bool IsLive => Snapshot.IsLive;

    /// <summary>
    /// The one render function.
    /// Composes the reading from the session, pushes it into every card that reads one, renders the cards that do
    /// not, and re-asks each action whether it is available.
    /// Safe to run twice: the resolve it reconciles is skipped while the running pipeline's settings have not
    /// moved, and a card whose input did not change does not re-render.
    /// </summary>
    public void Apply()
    {
        var reading = _held.Fill(BroadcastSnapshot.Of(_session.Publish, _session.Stats, _session.Relay));
        Snapshot = reading;

        // Reconciled from the render pass rather than performed by it: the pass states what it wants described,
        // and the converge decides whether anything has to be asked.
        Describe();

        Stats.Snapshot = reading;
        Preview.Snapshot = reading;
        Plots.Snapshot = reading;
        Plots.Samples = _session.Samples;
        Plots.RelaySamples = _session.RelaySamples;

        Config.Reported = Rows(_form, _session.Words);
        Config.IsLive = reading.IsLive;
        Viewers.Reported = Watching(_session.Relay, reading.Stream);
        Viewers.Readers = reading.Viewers;
        Viewers.IsLive = reading.IsLive;
        Log.Recorded = Recorded(_session.Exits, Audience.Of(_session.RelaySamples, reading.Stream));
        TestStreams.Reported = _session.TestStreams;

        Config.Apply();
        Viewers.Apply();
        TestStreams.Apply();
        Log.Apply();

        // Rendered as well as told its reading, unlike the cards above: the preview also reads what is decoding,
        // which the composed reading does not carry, so a pass where only that moved would write nothing into it.
        Preview.Apply();

        HasRefusal = Refusal.Length > 0;

        PauseCommand.Refresh();
        ForceKeyframeCommand.Refresh();
        ReconnectCommand.Refresh();
        StopCommand.Refresh();

        Assert.That(
            Stats.Snapshot == reading && Preview.Snapshot == reading && Plots.Snapshot == reading,
            "every card on the screen describes one reading", reading.Elapsed);
        Assert.That(
            !PauseCommand.CanExecute(null) && !ForceKeyframeCommand.CanExecute(null) && !ReconnectCommand.CanExecute(null),
            "an action the contract has no method for is never pressable");
        Assert.That(UnbackedActions.Length > 0, "an inert action says why");
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }

    // --- The running configuration --------------------------------------------------

    /// <summary>
    /// Converges the described configuration onto the running pipeline's settings, resolving a form for them
    /// unless one has already been resolved for exactly these.
    /// The card needs a label and a shorthand per group and both are the backend's, so composing either here
    /// would let this screen and the setup step describe one configuration two ways.
    /// </summary>
    private void Describe()
    {
        // The live state carries the two groups the running pipeline was built from, and a resolve takes all three,
        // so the resolve fills the viewer group from the defaults.
        // Hence Rows leaving that group out: how this machine watches is no part of what it publishes.
        var live = _session.Publish?.Live;
        var settings = live is null ? null : new Settings { Publish = live.Publish, Relay = live.Relay };

        if (settings is null)
        {
            _described = null;
            _form = null;
            return;
        }

        if (settings.Equals(_described))
        {
            return;
        }

        _described = settings;

        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = new CancellationTokenSource();

        _ = DescribeAsync(settings.Clone(), _cancel.Token);
    }

    private async Task DescribeAsync(Settings settings, CancellationToken cancellation)
    {
        try
        {
            var form = await _backend.ResolveFormAsync(settings, cancellation).ConfigureAwait(false);

            _dispatch(() =>
            {
                if (cancellation.IsCancellationRequested)
                {
                    return;
                }

                _form = form;
                Apply();
            });
        }
        catch (OperationCanceledException)
        {
            // The pipeline moved off these settings while they were being described.
        }
        catch (BackendUnavailableException)
        {
            // The session's own reconnect reports the absence, so a second sentence here says nothing new.
            // Forgetting what was asked lets the next pass ask again once the backend answers.
            _dispatch(() => _described = null);
        }
    }

    /// <summary>
    /// Configuration rows: one per group of the resolved form, read back rather than edited.
    /// The grouping and the values are the form's, the heading and the shorthand composed here out of the draft
    /// the form carried.
    ///
    /// A group with no shorthand is left out rather than drawn empty, the relay ports settling on numbers that say
    /// nothing without their labels.
    ///
    /// So is the group about receiving.
    /// The live state does not carry it and the resolve fills it from the defaults, so the row would be a figure
    /// about the machine under a heading about the stream (<c>Features/Fields/Model/GroupPlacement.cs</c>).
    /// </summary>
    private static IReadOnlyList<ConfigRow> Rows(Form? form, Vocabulary words)
    {
        if (form is null)
        {
            return [];
        }

        var rows = new List<ConfigRow>(form.Groups.Count);
        foreach (var group in form.Groups)
        {
            if (GroupPlacement.InViewer(group.Key))
            {
                continue;
            }

            var summary = words.Shorthand(group, form.Settings);
            if (summary.Length > 0)
            {
                rows.Add(new ConfigRow(Copy.Fields.Group(group.Key).Title, summary));
            }
        }

        return rows;
    }

    /// <summary>
    /// Viewer rows: one per reader the relay named on this stream's path, in the relay's own order.
    /// Nothing is sorted or ranked here: a table that promoted the struggling viewer would move, every pass,
    /// the row a reader had learned the position of.
    /// No path in the snapshot, an unreachable relay and nothing publishing all come out as no rows, and the count
    /// beside this says which.
    /// </summary>
    private static IReadOnlyList<ViewerRow> Watching(RelayStatus? relay, string stream)
    {
        var path = BroadcastSnapshot.PathOf(relay, stream);
        if (path is null)
        {
            return [];
        }

        var rows = new List<ViewerRow>(path.ReaderRoster.Count);
        foreach (var reader in path.ReaderRoster)
        {
            rows.Add(ViewerRow.Of(reader));
        }

        return rows;
    }

    /// <summary>
    /// Log lines, newest first: what the event stream reported ended, and who started or stopped watching.
    /// Interleaved by time rather than split into two lists, the useful reading of a pipeline that died being
    /// the viewers that left in the same second.
    /// Ordered here rather than in the session, which holds both as they happened while the card reads them
    /// as news.
    /// </summary>
    private static IReadOnlyList<LogLine> Recorded(
        IReadOnlyList<SessionExit> exits, IReadOnlyList<AudienceChange> audience)
    {
        var entries = new List<(DateTimeOffset At, LogLine Line)>(exits.Count + audience.Count);
        foreach (var exit in exits)
        {
            entries.Add((exit.At, LogLine.Of(exit)));
        }

        foreach (var change in audience)
        {
            entries.Add((change.At, LogLine.Of(change)));
        }

        return entries.OrderByDescending(entry => entry.At).Select(entry => entry.Line).ToList();
    }

    // --- The effects ----------------------------------------------------------------

    /// <summary>
    /// Opens the run log of the newest thing that ended, or the folder holding them where nothing has.
    /// Both are the backend's, the files being on its machine, which is also why the card waits on the call.
    /// Nothing is written on the way out of either effect: the reply carries no state and what the stream became
    /// arrives on the event stream, so every window learns it the same way.
    /// </summary>
    private Task OpenLogAsync()
    {
        Request(BroadcastAction.OpenFullLog);

        var newest = "";
        foreach (var exit in _session.Exits)
        {
            if (exit.Info.LogPath.Length > 0)
            {
                newest = exit.Info.LogPath;
            }
        }

        return PerformAsync(newest.Length > 0
            ? cancellation => _backend.OpenLogAsync(newest, cancellation)
            : _backend.OpenLogsFolderAsync);
    }

    /// <summary>
    /// Asks the backend for one effect and shows its refusal where there is one.
    /// A refusal is an environment condition carrying prose written for a person, shown as it stands rather than
    /// mapped to a sentence of this screen's (<c>docs/ipc-api.md</c>, "Errors").
    /// A success clears what the last one left, the render function's off branch applied to a string.
    /// </summary>
    private async Task PerformAsync(Func<CancellationToken, Task> effect)
    {
        try
        {
            await effect(default).ConfigureAwait(false);
            Refused("");
        }
        catch (BackendUnavailableException e)
        {
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    private void Refused(string reason)
    {
        _dispatch(() =>
        {
            Refusal = reason;
            _report(reason);
            Apply();
        });
    }

    private void Request(BroadcastAction action)
    {
        Assert.That(Enum.IsDefined(action), "a control asks for an action this screen names", (int)action);

        ActionRequested?.Invoke(action);
    }
}
