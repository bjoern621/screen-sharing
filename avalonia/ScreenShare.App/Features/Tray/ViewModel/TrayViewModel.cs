using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Insights.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Tray.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Tray.ViewModel;

/// <summary>
/// The tray's state: one menu, derived from the window's own controls rather than from a second gate.
///
/// The commit row presses the review's commit and the insights screen's stop, so the wait, the guard and
/// the refusal surface stay one each, and a refusal lands where the window already shows it.
/// The preset rows are the rail card's, applied through the card's own commands.
/// Nothing here decides anything: which effect a press is comes off the running state at the press,
/// the way every commit reads it (<see cref="PublishGate.CommitFor"/>).
///
/// <see cref="Apply"/> is the one render function.
/// The shell's pass drives it for the session's state; the review, the stop command and the card's rows
/// announce their own moves, so the menu follows a form or a store the window read while hidden.
/// <see cref="TrayMenu"/> compares by content, so an unchanged pass notifies nothing.
/// </summary>
public sealed class TrayViewModel : Observable
{
    /// <summary>Bound wait for the stops a quit runs first, so a wedged backend cannot hold the exit.</summary>
    private static readonly TimeSpan StopBudget = TimeSpan.FromSeconds(2);

    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly SetupViewModel _setup;
    private readonly InsightsViewModel _insights;
    private readonly Func<bool> _ownsBackend;
    private readonly Func<CancellationToken, Task> _part;
    private readonly Action<Action> _dispatch;

    private TrayMenu _menu = TrayMenu.Unread;

    /// <param name="ownsBackend">
    /// Whether this shell has a backend of its own running, read at the press.
    /// A function rather than a value: the backend is started lazily, on the first connect that finds
    /// nothing listening (<c>Backend/BackendProcess.cs</c>).
    /// </param>
    /// <param name="part">
    /// Closes what the window alone holds open on the backend, the grid's decodes and the preview's,
    /// answering once the backend has (<c>Features/Shell/ViewModel/ShellViewModel.cs</c>).
    /// Run on every quit, whichever backend runs them: nothing draws them once the process is gone.
    /// </param>
    public TrayViewModel(
        IBackend backend,
        Session session,
        SetupViewModel setup,
        InsightsViewModel insights,
        Func<bool> ownsBackend,
        Func<CancellationToken, Task> part,
        Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a tray asks the backend to stop the stream a quit ends");
        Assert.NotNull(session, "a tray reads what is publishing off the session");
        Assert.NotNull(setup, "a tray presses the setup flow's own commit");
        Assert.NotNull(insights, "a tray presses the insights screen's own stop");
        Assert.NotNull(ownsBackend, "a tray asks whether this shell has a backend of its own to stop");
        Assert.NotNull(part, "a tray closes the window's decodes before a quit");
        Assert.NotNull(dispatch, "a tray needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _setup = setup;
        _insights = insights;
        _ownsBackend = ownsBackend;
        _part = part;
        _dispatch = dispatch;

        QuitCommand = new PendingCommand(QuitAsync, dispatch);

        // Everything the menu draws beside the session's own state, whose pass the shell drives:
        // the review's gate and label, the stop's liveness, and the card's preset rows.
        // Each announces its own moves, so the menu follows them while the window is hidden.
        setup.Review.PropertyChanged += (_, _) => Apply();
        insights.StopCommand.Changed += Apply;
        setup.Rail.Presets.Rows.CollectionChanged += (_, _) => Apply();
        setup.Rail.Presets.Builtin.CollectionChanged += (_, _) => Apply();

        Apply();
    }

    /// <summary>Raised on a press asking for the window, which is the host's to show.</summary>
    public event Action? OpenRequested;

    /// <summary>Raised on the UI loop once a quit's stop has been attempted, which is the host's to perform.</summary>
    public event Action? QuitRequested;

    /// <summary>The menu's whole state, written by <see cref="Apply"/> alone.</summary>
    public TrayMenu Menu { get => _menu; private set => Set(ref _menu, value); }

    /// <summary>
    /// Ends the app: the window's decodes and the stream first, then <see cref="QuitRequested"/>.
    /// Waits rather than going inert, each stop being a round trip.
    /// </summary>
    public PendingCommand QuitCommand { get; }

    public void Open() => OpenRequested?.Invoke();

    /// <summary>
    /// The tray's one commit: starts sharing where nothing runs, ends the stream where one does.
    /// Which of the two is read off the running state at the press rather than off the menu that was drawn,
    /// a stream being able to start or end in between.
    /// </summary>
    public void Commit()
    {
        if (_session.Publish?.Live is not null)
        {
            _insights.StopCommand.Execute(null);
            return;
        }

        _setup.Review.StartSharingCommand.Execute(null);
    }

    /// <summary>
    /// Writes one preset into the draft through the card's own row, and commits:
    /// the stream starts on it, or restarts where one is on the air, the commit reading which
    /// off the running state.
    /// The same press as the strip menu's preset rows, so one gesture means one thing everywhere.
    /// The row is looked up at the press: a store re-read can land between the render and the pick,
    /// and a row missing from the card does nothing, like a name the store lost does on the card.
    /// </summary>
    public void UsePreset(TrayPresetEntry entry)
    {
        Assert.NotNull(entry, "picking a preset names the row that was picked");

        var applied = entry.Kind switch
        {
            TrayPresetKind.Builtin => ApplyBuiltin(entry.Id),
            TrayPresetKind.Saved => ApplySaved(entry.Id),
            _ => Assert.Never<bool>("unexpected preset kind", (int)entry.Kind),
        };

        if (applied)
        {
            _setup.Review.StartSharingCommand.Execute(null);
        }
    }

    /// <summary>
    /// The one render function.
    /// Derives the whole menu on every pass; an unchanged one compares equal and notifies nothing.
    /// </summary>
    public void Apply()
    {
        var live = _session.Publish?.Live is not null;
        var card = _setup.Rail.Presets;

        Menu = new TrayMenu
        {
            IsLive = live,
            CommitLabel = live ? InsightsViewModel.StopLabel : CommitCopy.Of(PublishCommit.Start).Label,

            // The command asked rather than a gate composed here, so the row and the press cannot disagree.
            CanCommit = live
                ? _insights.StopCommand.CanExecute(null)
                : _setup.Review.StartSharingCommand.CanExecute(null),

            Presets = EntriesOf(card),
        };

        Assert.That(
            Menu.Presets.Count == card.Builtin.Count + card.Rows.Count,
            "a tray row per preset the card lists", Menu.Presets.Count);
    }

    /// <summary>Built-in rows first, saved rows after them, both in the card's own order.</summary>
    private static IReadOnlyList<TrayPresetEntry> EntriesOf(PresetsViewModel card)
    {
        var entries = new List<TrayPresetEntry>(card.Builtin.Count + card.Rows.Count);

        foreach (var row in card.Builtin)
        {
            entries.Add(new TrayPresetEntry
            {
                Kind = TrayPresetKind.Builtin,
                Id = row.Key,
                Name = row.Name,
                IsCurrent = row.IsCurrent,
                IsReachable = row.IsReachable,
            });
        }

        foreach (var row in card.Rows)
        {
            entries.Add(new TrayPresetEntry
            {
                Kind = TrayPresetKind.Saved,
                Id = row.Name,
                Name = row.Name,
                IsCurrent = row.IsCurrent,
                IsReachable = true,
            });
        }

        return entries;
    }

    /// <summary>Answers whether the draft moved, which is what decides whether a live stream restarts.</summary>
    private bool ApplyBuiltin(string key)
    {
        var row = _setup.Rail.Presets.Builtin.FirstOrDefault(row => row.Key == key);
        if (row is null || !row.IsReachable)
        {
            return false;
        }

        row.Apply.Execute(null);
        return true;
    }

    private bool ApplySaved(string name)
    {
        var row = _setup.Rail.Presets.Rows.FirstOrDefault(row => row.Name == name);
        if (row is null)
        {
            return false;
        }

        row.Apply.Execute(null);
        return true;
    }

    /// <summary>
    /// Closes the window's decodes and stops the stream, side by side under one budget, then asks the host
    /// to shut down.
    /// The decodes close on every quit, being the shell's alone.
    /// Only a backend this shell started has its stream stopped: it dies with the shell either way, and the stop
    /// lets the relay drop the session now rather than at the lease sweep.
    /// One left running keeps its stream (<c>Backend/BackendProcess.cs</c>).
    /// A stop that failed or ran out the budget still quits: the exit is what was asked for, and the kill on exit
    /// takes the pipeline with it.
    /// </summary>
    private async Task QuitAsync()
    {
        using var bound = new CancellationTokenSource(StopBudget);

        var part = Attempt(_part(bound.Token));
        var stop = _session.Publish?.Live is not null && _ownsBackend()
            ? Attempt(_backend.StopPublishAsync(bound.Token))
            : Task.CompletedTask;
        await Task.WhenAll(part, stop).ConfigureAwait(false);

        _dispatch(() => QuitRequested?.Invoke());
    }

    /// <summary>Waits one stop out. A backend that cannot be reached has nothing to stop, and a budget can run out.</summary>
    private static async Task Attempt(Task stop)
    {
        try
        {
            await stop.ConfigureAwait(false);
        }
        catch (BackendUnavailableException)
        {
        }
        catch (OperationCanceledException)
        {
        }
    }
}
