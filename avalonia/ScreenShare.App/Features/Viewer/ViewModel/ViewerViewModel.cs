using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// What the relay is carrying, and what this machine is watching of it.
///
/// <b>This is a roster and not a tile grid, and the difference is the frame channel.</b> A grid
/// shows decoded pictures and per-tile decode figures; both need frames, and frames deliberately
/// do not cross the control API - they are a second channel of shared GPU handles that does not
/// exist yet (<c>docs/ipc-api.md</c>, and <c>avalonia/README.md</c>, "What is not settled yet").
/// Everything the control API does carry about a session is here and is real: which streams the
/// relay has, whether each is being served, what it says they carry, how many readers each has,
/// what each is ingesting, and which of them this machine has viewers open on. The grid layout,
/// the spotlight and the per-tile menus were removed rather than left drawing mockup figures
/// beside real ones.
///
/// There is no longer a button for the GTK4 grid window either, and its absence is the contract's
/// rather than this module's. How a viewer arranges what it receives is the shell's whole job, so
/// the backend describes no grid to open, close or report the state of (<c>docs/ipc-api.md</c>).
/// When frames do cross, the window that shows them is this module's to build and not one it asks
/// for by name.
///
/// <b>It holds no snapshot of its own.</b> <see cref="Backend.Session"/> owns the running state
/// and <see cref="Apply"/> reads it through on every pass, so the list and the status band cannot
/// disagree about what is on the relay.
///
/// The legs a stream can be opened on come from the backend too: they are the options of the
/// form's watch-leg field, so this module holds no list of protocols. Whether a particular leg
/// can carry a particular stream is answered by the backend when the viewer is opened, and its
/// refusal is shown as it stands - the relay snapshot can be older than the stream, so greying a
/// leg here from a stale format would refuse a viewer that would have worked.
/// </summary>
public sealed class ViewerViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;
    private readonly Dictionary<string, StreamRowViewModel> _rows = [];

    /// <summary>
    /// The legs the backend offered, and empty until the settings have been resolved once. An
    /// empty list is a roster whose rows offer nothing rather than one that invented a protocol.
    /// </summary>
    private IReadOnlyList<WatchLeg> _legs = [];

    /// <summary>Whether the legs have been asked for, so a failed read is retried and a good one is not repeated.</summary>
    private bool _askedLegs;

    /// <param name="dispatch">
    /// Hands work to the UI loop. Every answer to an effect lands on whichever thread the
    /// transport completed on, and everything this writes is read by a binding that only
    /// tolerates being written from one.
    /// </param>
    public ViewerViewModel(IBackend backend, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a viewer asks the backend to open and close viewers");
        Assert.NotNull(session, "a viewer renders the session's running state");
        Assert.NotNull(dispatch, "a viewer needs a UI loop to marshal an answer back to");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;

        Streams = [];

        Apply();
    }

    // --- Outputs --------------------------------------------------------------------

    private string _shownSummary = "";
    private string _notice = "";
    private bool _hasNotice;
    private string _refusal = "";
    private bool _hasRefusal;
    private bool _hasStreams;

    /// <summary>The streams the relay is carrying, in the order it listed them.</summary>
    public ObservableCollection<StreamRowViewModel> Streams { get; }

    /// <summary>How much of what the relay carries this machine is watching. The status band repeats it.</summary>
    public string ShownSummary { get => _shownSummary; private set => Set(ref _shownSummary, value); }

    /// <summary>
    /// What the relay costs, as the status band prints it. A list rather than named slots, so
    /// what this destination reports stays this destination's business.
    /// </summary>
    public IReadOnlyList<string> Figures { get; private set; } = [];

    /// <summary>Why the list is empty, empty while it is not. It separates an unread relay from an idle one.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>The backend's own sentence when it refused to open or close a viewer, empty otherwise.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    public bool HasStreams { get => _hasStreams; private set => Set(ref _hasStreams, value); }

    /// <summary>
    /// The status band's affordance. It names what this screen affords rather than what a tile
    /// grid would, because this screen has no tiles.
    /// </summary>
    public string Hint => "Opening a viewer launches an external player";

    /// <summary>The list's leading label.</summary>
    public string ShowingLabel => "On the relay";

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function. Reads the session through on every pass and keeps no copy of what
    /// it found. Safe to run twice: rows are reused by stream name and each runs its own
    /// idempotent pass, so an unchanged relay snapshot fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it: the pass states that it
        // wants legs to offer, and the converge decides whether anything has to be asked.
        AskLegs();

        var relay = _session.Relay;
        var rows = Rows(relay, _session.Watching);

        foreach (var row in rows)
        {
            Of(row.Name).Apply(row, _legs);
        }

        Reconcile.Onto(Streams, rows.Select(row => Of(row.Name)).ToList());

        HasStreams = Streams.Count > 0;
        Notice = HasStreams ? "" : NoticeFor(relay);
        HasNotice = Notice.Length > 0;

        var watched = rows.Count(row => row.IsWatched);
        ShownSummary = HasStreams ? $"{watched} of {rows.Count} streams watched" : "";
        Figures = FiguresFor(rows);

        HasRefusal = Refusal.Length > 0;

        Assert.That(Streams.Count == rows.Count, "a row per stream on the relay", Streams.Count, rows.Count);
        Assert.That(HasStreams == (Notice.Length == 0), "a list and the sentence standing in for it are never both on screen", HasStreams);
        Assert.That(HasNotice == (Notice.Length > 0), "the notice and its text agree", HasNotice);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }

    /// <summary>
    /// One row per relay path, carrying the legs this machine already has open on it. The join is
    /// on the name the backend uses on both sides, so nothing here has to know what a transport
    /// or a format is.
    /// </summary>
    private static IReadOnlyList<StreamRow> Rows(RelayStatus? relay, IReadOnlyList<WatchKey> watching)
    {
        if (relay is null || !relay.Reachable)
        {
            return [];
        }

        var rows = new List<StreamRow>(relay.Paths.Count);
        foreach (var path in relay.Paths)
        {
            var open = new List<string>();
            foreach (var key in watching)
            {
                if (key.StreamName == path.Name)
                {
                    open.Add(key.Transport);
                }
            }

            rows.Add(new StreamRow
            {
                Name = path.Name,
                IsReady = path.Ready,
                Tracks = path.Tracks,
                Format = path.Format,
                Readers = path.Readers,
                InMbps = path.InMbps,
                WatchedOn = open,
            });
        }

        return rows;
    }

    /// <summary>
    /// Why there is nothing to list. Three states the reader has to be able to tell apart: the
    /// relay has not been read, it could not be reached - and then it says why, in its own words
    /// - or it is up and carrying nothing.
    /// </summary>
    private string NoticeFor(RelayStatus? relay)
    {
        if (relay is null)
        {
            return _session.Unavailable.Length > 0
                ? _session.Unavailable
                : "Reading what the relay is carrying.";
        }

        if (!relay.Reachable)
        {
            return relay.Error.Length > 0 ? relay.Error : "The relay could not be reached.";
        }

        return "The relay is up and carrying nothing.";
    }

    /// <summary>
    /// What the band prints: what the relay is ingesting in total, and how many readers it has
    /// across every path. Both are the relay's own figures rather than this machine's - this
    /// shell receives nothing, so a receive rate here would be a number about somebody else.
    /// </summary>
    private static IReadOnlyList<string> FiguresFor(IReadOnlyList<StreamRow> rows)
    {
        if (rows.Count == 0)
        {
            return [];
        }

        var ingest = rows.Where(row => row.IsReady).Sum(row => row.InMbps);
        var readers = rows.Sum(row => row.Readers);

        return [$"relay in {ingest:0.0} Mb/s", $"{readers} readers"];
    }

    private StreamRowViewModel Of(string name)
    {
        if (_rows.TryGetValue(name, out var row))
        {
            return row;
        }

        row = new StreamRowViewModel(name, WatchAsync);
        _rows[name] = row;
        return row;
    }

    // --- The effects ----------------------------------------------------------------

    /// <summary>
    /// Asks the backend which legs a viewer can be opened on, once.
    ///
    /// They are the options of the form's watch-leg field, resolved against the settings the
    /// backend holds. That is a read of the vocabulary rather than of a per-stream verdict, which
    /// is why one answer serves every row: what a leg is called is the same for all of them, and
    /// whether one carries a given stream is answered when that viewer is opened.
    /// </summary>
    private void AskLegs()
    {
        if (_askedLegs)
        {
            return;
        }

        _askedLegs = true;
        _ = AskLegsAsync();
    }

    private async Task AskLegsAsync()
    {
        try
        {
            var settings = await _backend.SettingsAsync().ConfigureAwait(false);
            var form = await _backend.ResolveFormAsync(settings).ConfigureAwait(false);

            _dispatch(() =>
            {
                _legs = LegsOf(form);
                Apply();
            });
        }
        catch (BackendUnavailableException)
        {
            // The session's own reconnect reports the absence. Forgetting that they were asked
            // for is what lets the next pass ask again once the backend answers.
            _dispatch(() => _askedLegs = false);
        }
        catch (OperationCanceledException)
        {
            _dispatch(() => _askedLegs = false);
        }
    }

    /// <summary>
    /// The watch legs a form offers, read off the field the contract names for them. A field the
    /// form does not carry leaves the roster offering nothing rather than a protocol guessed here.
    /// </summary>
    private static IReadOnlyList<WatchLeg> LegsOf(Form form)
    {
        foreach (var group in form.Groups)
        {
            foreach (var field in group.Fields)
            {
                if (field.Key != "watch_transport")
                {
                    continue;
                }

                // The roster is the backend's and the names are this side's: which legs a
                // viewer can be opened on is a fact, and what each protocol is called on a
                // toggle is a decision about this screen.
                return field.Options
                    .Select(option => new WatchLeg(option.Value, Copy.Words.Transport(option.Value)))
                    .ToList();
            }
        }

        return [];
    }

    /// <summary>
    /// Opens or closes one external viewer. Nothing is written here on the way out: the roster is
    /// the backend's list and this reads it again when the event stream says it moved.
    /// </summary>
    private async Task WatchAsync(string stream, string transport, bool open)
    {
        try
        {
            if (open)
            {
                await _backend.StopWatchAsync(stream, transport).ConfigureAwait(false);
            }
            else
            {
                await _backend.StartWatchAsync(stream, transport).ConfigureAwait(false);
            }

            Refused("");

            // The backend announces a viewer that ended and not one that started, so the roster
            // is re-read here. It is a read of the backend's own list rather than an edit of a
            // copy, which is what keeps one opinion about what is open.
            var watching = await _backend.WatchingAsync().ConfigureAwait(false);
            _session.Adopt(watching);
        }
        catch (BackendUnavailableException e)
        {
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Records what the backend said about the last effect and re-renders, on the UI loop. An
    /// empty reason is a success, which clears whatever the last failure left - the render
    /// function's usual property applied to a string.
    /// </summary>
    private void Refused(string reason)
    {
        _dispatch(() =>
        {
            Refusal = reason;
            Apply();
        });
    }
}
