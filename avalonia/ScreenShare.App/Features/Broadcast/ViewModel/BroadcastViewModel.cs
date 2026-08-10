using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.ConfigCard.ViewModel;
using ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Nudge.ViewModel;
using ScreenShare.App.Features.Broadcast.Plots.ViewModel;
using ScreenShare.App.Features.Broadcast.Preview.ViewModel;
using ScreenShare.App.Features.Broadcast.SessionLog.ViewModel;
using ScreenShare.App.Features.Broadcast.ViewerTable.ViewModel;
using ScreenShare.App.Features.Fields.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.ViewModel;

/// <summary>
/// The broadcast screen: the live overview, and read-only about everything else.
///
/// The rule it exists to enforce is that a control appears in exactly one window. This view
/// model therefore owns only actions that are safe while a stream is running, and hands
/// configuration to a card that shows it and cannot write it. The single way out is
/// <see cref="BroadcastAction.EditInSetup"/>, which navigates rather than editing here.
///
/// <b>It holds no reading of its own.</b> <see cref="Backend.Session"/> owns the running state
/// and this reads it through on every pass, so a card cannot go on describing a stream that has
/// stopped (<c>docs/development-principles.md</c>, "State has one owner"). <see cref="Apply"/>
/// is the one render function and it pushes one composed reading into every card, so two cards
/// can never disagree about the stream they are describing.
///
/// <b>Three of the design's controls are inert, and they stay on screen greyed.</b> Pause,
/// force keyframe and reconnect name effects the control contract does not have - a method that
/// is neither a read nor one of the listed effects does not belong on that service
/// (<c>docs/ipc-api.md</c>, "The rule"), and none of the three is on it. Removing the buttons
/// would hide that the capability is missing; wiring them to something else would be worse. So
/// they are drawn, disabled, carrying why - the treatment the settings form gives a concept the
/// current combination blocks.
///
/// Stop is the one that is real, and pressing it writes nothing here: the reply says nothing
/// and what the state became arrives on the event stream, which is the one path into the
/// display.
/// </summary>
public sealed class BroadcastViewModel : Observable
{
    /// <summary>
    /// Why the three header actions are inert. One sentence for all three, because it is one
    /// fact about the backend rather than three about the buttons.
    /// </summary>
    private const string UnbackedReason =
        "The control contract has no pause, keyframe or reconnect effect: it carries the reads "
        + "a screen draws from and the few effects a user asks for by name.";

    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>
    /// What the shell does with a refusal the backend answered with. Set from outside, so a
    /// screen built with no shell around it - which is every test - still renders and still
    /// presses buttons.
    /// </summary>
    private Action<string> _report = static _ => { };

    /// <summary>
    /// The settings the config card was last resolved against, so an unchanged pipeline is not
    /// re-resolved on every event. A pipeline emits a sample per second and its settings do not
    /// move while it runs.
    /// </summary>
    private Settings? _described;

    /// <summary>What that resolve answered with, and null until one has landed.</summary>
    private Form? _form;

    /// <summary>Cancels a resolve in flight when the running pipeline moves off it.</summary>
    private CancellationTokenSource? _cancel;

    /// <summary>What a control on this screen asked for. Raised once per press, never during a render.</summary>
    public event Action<BroadcastAction>? ActionRequested;

    /// <param name="dispatch">
    /// Hands work to the UI loop. The answer to an effect and to a resolve both land on whichever
    /// thread the transport completed on, and everything this writes is read by a binding that
    /// only tolerates being written from one.
    /// </param>
    public BroadcastViewModel(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a broadcast screen asks the backend to act");
        Assert.NotNull(session, "a broadcast screen renders the session's running state");
        Assert.NotNull(dispatch, "a broadcast screen needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        Stats = new HeaderStatsViewModel();

        // The one card here that asks the backend for something. It receives this machine's
        // own stream back off the relay, so it needs the seam and the running state rather
        // than the composed reading alone (Preview/ViewModel/PreviewViewModel.cs).
        Preview = new PreviewViewModel(backend, session, dispatch);
        Nudge = new NudgeViewModel();
        Config = new ConfigCardViewModel();
        Viewers = new ViewerTableViewModel();
        Plots = new PlotsViewModel();
        Log = new SessionLogViewModel(OpenLogAsync, dispatch);

        // The three the contract has no method for. They are constructed unpressable rather
        // than disabled by a later pass, so there is no instant in which one of them works.
        PauseCommand = new DelegateCommand(() => Request(BroadcastAction.Pause), static () => false);
        ForceKeyframeCommand = new DelegateCommand(() => Request(BroadcastAction.ForceKeyframe), static () => false);
        ReconnectCommand = new DelegateCommand(() => Request(BroadcastAction.Reconnect), static () => false);

        // Ending a stream crosses to the backend and the encoder it has to bring down, so the
        // button waits on the answer rather than sitting still - and the command refuses the
        // second press a reader gives a control that looks like it did nothing.
        StopCommand = new PendingCommand(() => PerformAsync(_backend.StopPublishAsync), dispatch, () => IsLive);

        // The one escape hatch the cards still raise as news. Opening the log is an effect the
        // card runs itself, because a card that waits on a call has to be the thing that knows
        // the call is running.
        Config.EditRequested += () => Request(BroadcastAction.EditInSetup);

        Apply();
    }

    /// <summary>
    /// Hands the screen somewhere to put a refusal the backend answered with. Idempotent, and
    /// rendering after it so a screen that already has one shows it on attach.
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

    public NudgeViewModel Nudge { get; }

    public ConfigCardViewModel Config { get; }

    public ViewerTableViewModel Viewers { get; }

    public PlotsViewModel Plots { get; }

    public SessionLogViewModel Log { get; }

    public DelegateCommand PauseCommand { get; }

    public DelegateCommand ForceKeyframeCommand { get; }

    public DelegateCommand ReconnectCommand { get; }

    /// <summary>The one red control on the screen, and the only one that ends the stream.</summary>
    public PendingCommand StopCommand { get; }

    /// <summary>
    /// The reading every card on this screen describes, composed from the session's whole states
    /// on each pass. An output rather than an input now: nothing writes it from outside, because
    /// the backend is the only thing that knows it.
    /// </summary>
    public BroadcastSnapshot Snapshot { get => _snapshot; private set => Set(ref _snapshot, value); }

    /// <summary>Why the three header actions are inert, for the tooltip that carries it.</summary>
    public string UnbackedActions => UnbackedReason;

    /// <summary>The backend's own sentence when it refused something this screen asked for, empty otherwise.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    /// <summary>
    /// Read through rather than mirrored, so a command asked twice in one pass cannot answer
    /// from a stale copy of the reading.
    /// </summary>
    private bool IsLive => Snapshot.IsLive;

    /// <summary>
    /// The one render function. Composes the reading from the session, pushes it into every card
    /// that reads one, calls the render function of every card that does not, and re-asks each
    /// action whether it is still available.
    ///
    /// Safe to run twice: the resolve it reconciles is skipped when the running pipeline's
    /// settings have not moved, a card whose input did not change does not re-render, and the
    /// ones that do are idempotent themselves.
    /// </summary>
    public void Apply()
    {
        var reading = BroadcastSnapshot.Of(_session.Publish, _session.Stats, _session.Relay);
        Snapshot = reading;

        // Reconciled from the render pass rather than performed by it, the same arrangement the
        // setup flow has with its resolve: the pass states what it wants described and the
        // converge decides whether anything has to be asked.
        Describe();

        Stats.Snapshot = reading;
        Preview.Snapshot = reading;
        Nudge.Snapshot = reading;
        Plots.Snapshot = reading;
        Plots.Samples = _session.Samples;
        Plots.RelaySamples = _session.RelaySamples;

        Config.Reported = Rows(_form, _session.Words);
        Viewers.Reported = Watching(_session.Relay, reading.Stream);
        Viewers.Readers = reading.Viewers;
        Log.Recorded = Recorded(_session.Exits);

        Config.Apply();
        Viewers.Apply();
        Log.Apply();

        // Rendered as well as told its reading, unlike the cards above it: the preview also
        // reads what is decoding, which is a state the composed reading does not carry, so a
        // pass where only that moved would otherwise write nothing into it.
        Preview.Apply();

        HasRefusal = Refusal.Length > 0;

        PauseCommand.Refresh();
        ForceKeyframeCommand.Refresh();
        ReconnectCommand.Refresh();
        StopCommand.Refresh();

        Assert.That(
            Stats.Snapshot == reading && Preview.Snapshot == reading
            && Nudge.Snapshot == reading && Plots.Snapshot == reading,
            "every card on the screen describes one reading", reading.Elapsed);
        Assert.That(
            !PauseCommand.CanExecute(null) && !ForceKeyframeCommand.CanExecute(null) && !ReconnectCommand.CanExecute(null),
            "an action the contract has no method for is never pressable");
        Assert.That(UnbackedActions.Length > 0, "an inert action says why");
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }

    // --- The running configuration --------------------------------------------------

    /// <summary>
    /// Converges the described configuration onto the running pipeline's settings: resolves a
    /// form for them, unless one has already been resolved for exactly these.
    ///
    /// The card needs a label and a shorthand per group, and both are the backend's - the key is
    /// a group's title and the value its own summary. Composing either here would let this
    /// screen and the setup step describe one configuration in two ways.
    /// </summary>
    private void Describe()
    {
        // The live state carries the two groups the running pipeline was built from, and a
        // resolve takes all three. The viewer group is left absent and the resolve fills it
        // with the defaults, which is why the card leaves that group out entirely: how this
        // machine watches was never part of what it publishes, so a watch row here would be
        // describing the machine under a heading that says "this stream" (Rows).
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
            // The running pipeline moved off these settings while they were being described.
        }
        catch (BackendUnavailableException)
        {
            // The session's own reconnect reports the absence; a second sentence about it on the
            // same screen would say nothing new. Forgetting what was asked is what lets the next
            // pass ask again once the backend answers.
            _dispatch(() => _described = null);
        }
    }

    /// <summary>
    /// The configuration rows: one per group of the resolved form, read back rather than
    /// edited. The grouping and the values are the form's; the heading and the shorthand
    /// beside it are composed here, out of the draft the form carried.
    ///
    /// A group with nothing worth a line is left out rather than drawn empty - the relay
    /// ports settle on numbers that say nothing without their labels.
    ///
    /// So is the group about receiving. This card says what the running stream is, and how this
    /// machine watches is not part of that: the resolve fills that group from the defaults
    /// because the live state does not carry it, so the row would be a figure about the machine
    /// under a heading about the stream (<c>Features/Fields/Model/GroupPlacement.cs</c>).
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

            var summary = words.Shorthand(group.Key, form.Settings);
            if (summary.Length > 0)
            {
                rows.Add(new ConfigRow(Copy.Fields.Group(group.Key).Title, summary));
            }
        }

        return rows;
    }

    /// <summary>
    /// The viewer rows: one per reader the relay named on this stream's path, in the relay's own
    /// order. Nothing is sorted or ranked here - the roster's order is the relay's answer, and a
    /// table that put the struggling viewer first would be re-deciding, every pass, which row the
    /// reader's eye had already learned the position of.
    ///
    /// A stream with no path in the snapshot yet, an unreachable relay and nothing publishing all
    /// come out as no rows, and the card says which of them it is from the count beside this.
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
    /// The log lines: what the event stream reported ended, newest first. The order is reversed
    /// here rather than in the session, because the session holds them as they happened and the
    /// card reads them as news.
    /// </summary>
    private static IReadOnlyList<LogLine> Recorded(IReadOnlyList<SessionExit> exits)
    {
        var lines = new List<LogLine>(exits.Count);
        for (var i = exits.Count - 1; i >= 0; i--)
        {
            lines.Add(LogLine.Of(exits[i]));
        }

        return lines;
    }

    // --- The effects ----------------------------------------------------------------

    /// <summary>
    /// Opens the run log of the newest thing that ended, or the folder holding them where
    /// nothing has. Both are the backend's, because the files are on its machine - which is
    /// also why it is a call that can take a moment and the card waits on it.
    ///
    /// Nothing is written here on the way out of either effect: a reply carries no state and
    /// what the stream became arrives on the event stream, which is what stops the window that
    /// pressed the button and the window that did not from showing different things.
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
    ///
    /// A refusal is an environment condition and its message is prose written for a person, so
    /// it is shown as it stands rather than mapped to a sentence of this screen's
    /// (<c>docs/ipc-api.md</c>, "Errors"). A success clears whatever the last one left, which is
    /// the render function's usual property applied to a string.
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
