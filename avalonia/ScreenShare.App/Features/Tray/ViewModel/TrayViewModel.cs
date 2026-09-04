using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.Presets.ViewModel;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Tray.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Tray.ViewModel;

/// <summary>
/// The tray's state: one menu, derived from the window's own controls rather than from a second gate.
///
/// The commit row presses the review's commit and the broadcast screen's stop, so the wait, the guard and
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
    /// <summary>Bound wait for the stop a quit runs first, so a wedged backend cannot hold the exit.</summary>
    private static readonly TimeSpan StopBudget = TimeSpan.FromSeconds(2);

    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly SetupViewModel _setup;
    private readonly BroadcastViewModel _broadcast;
    private readonly Func<bool> _ownsBackend;
    private readonly Action<Action> _dispatch;

    private TrayMenu _menu = TrayMenu.Unread;

    /// <param name="ownsBackend">
    /// Whether this shell has a backend of its own running, read at the press.
    /// A function rather than a value: the backend is started lazily, on the first connect that finds
    /// nothing listening (<c>Backend/BackendProcess.cs</c>).
    /// </param>
    public TrayViewModel(
        IBackend backend,
        Session session,
        SetupViewModel setup,
        BroadcastViewModel broadcast,
        Func<bool> ownsBackend,
        Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a tray asks the backend to stop the stream a quit ends");
        Assert.NotNull(session, "a tray reads what is publishing off the session");
        Assert.NotNull(setup, "a tray presses the setup flow's own commit");
        Assert.NotNull(broadcast, "a tray presses the broadcast screen's own stop");
        Assert.NotNull(ownsBackend, "a tray asks whether this shell has a backend of its own to stop");
        Assert.NotNull(dispatch, "a tray needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _setup = setup;
        _broadcast = broadcast;
        _ownsBackend = ownsBackend;
        _dispatch = dispatch;

        QuitCommand = new PendingCommand(QuitAsync, dispatch);

        // Everything the menu draws beside the session's own state, whose pass the shell drives:
        // the review's gate and label, the stop's liveness, and the card's preset rows.
        // Each announces its own moves, so the menu follows them while the window is hidden.
        setup.Review.PropertyChanged += (_, _) => Apply();
        broadcast.StopCommand.Changed += Apply;
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
    /// Ends the app: stream first, then <see cref="QuitRequested"/>.
    /// Waits rather than going inert, the stop being a round trip.
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
            _broadcast.StopCommand.Execute(null);
            return;
        }

        _setup.Review.StartSharingCommand.Execute(null);
    }

    /// <summary>
    /// Writes one preset into the draft through the card's own row, and restarts a live stream on it.
    /// The row is looked up at the press: a store re-read can land between the render and the pick,
    /// and a row missing from the card does nothing, like a name the store lost does on the card.
    /// The restart is the review's own commit, <c>ApplyToStream</c> for a stream in force,
    /// so switching while live is the same press as applying from the window.
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

        if (applied && _session.Publish?.Live is not null)
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
            CommitLabel = live ? BroadcastViewModel.StopLabel : CommitCopy.Of(PublishCommit.Start).Label,

            // The command asked rather than a gate composed here, so the row and the press cannot disagree.
            CanCommit = live
                ? _broadcast.StopCommand.CanExecute(null)
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
    /// Stops the stream, then asks the host to shut down.
    /// Only a backend this shell started is stopped: it dies with the shell either way, and the stop lets
    /// the relay drop the session now rather than at the lease sweep.
    /// One left running keeps its stream (<c>Backend/BackendProcess.cs</c>).
    /// Bounded, and a stop that failed still quits: the exit is what was asked for, and the kill on exit
    /// takes the pipeline with it.
    /// </summary>
    private async Task QuitAsync()
    {
        if (_session.Publish?.Live is not null && _ownsBackend())
        {
            using var bound = new CancellationTokenSource(StopBudget);
            try
            {
                await _backend.StopPublishAsync(bound.Token).ConfigureAwait(false);
            }
            catch (BackendUnavailableException)
            {
                // A backend that cannot be reached has nothing to stop.
            }
            catch (OperationCanceledException)
            {
                // The budget ran out.
            }
        }

        _dispatch(() => QuitRequested?.Invoke());
    }
}
