using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One stream on the relay, as the list draws it: what it is, what it is carrying, and the legs
/// this machine can open a viewer on.
///
/// <b>Outputs only.</b> The row has no input of its own - a click runs one of the leg commands,
/// which asks the backend and waits for the event stream to say what happened, exactly as the
/// broadcast screen's stop does.
///
/// It is kept across passes and updated in place rather than rebuilt, so the legs' commands stay
/// the same instances and an expanded row does not collapse under the pointer every time the
/// relay is polled.
/// </summary>
public sealed class StreamRowViewModel : Observable
{
    private readonly Func<string, string, bool, Task> _watch;
    private readonly Dictionary<string, DelegateCommand> _toggles = [];

    /// <summary>The row's own view of which legs are open, read by the commands it hands out.</summary>
    private IReadOnlyList<string> _watchedOn = [];

    public StreamRowViewModel(string name, Func<string, string, bool, Task> watch)
    {
        Assert.That(name.Length > 0, "a stream row names the stream it stands for");
        Assert.NotNull(watch, "a stream row needs somewhere to ask for a viewer");

        Name = name;
        _watch = watch;
        Legs = [];
    }

    /// <summary>The stream name, carried and never parsed.</summary>
    public string Name { get; }

    // --- Outputs ------------------------------------------------------------------

    private string _detail = "";
    private string _tracks = "";
    private string _readers = "";
    private bool _isReady;
    private bool _isWatched;

    /// <summary>The legs a viewer can be opened on, in the order the backend offered them.</summary>
    public ObservableCollection<WatchLegViewModel> Legs { get; }

    /// <summary>The rate while the stream runs, and what it is doing otherwise.</summary>
    public string Detail { get => _detail; private set => Set(ref _detail, value); }

    /// <summary>The relay's own description of what the path carries.</summary>
    public string Tracks { get => _tracks; private set => Set(ref _tracks, value); }

    /// <summary>How many readers the relay counts, as the row prints it.</summary>
    public string Readers { get => _readers; private set => Set(ref _readers, value); }

    /// <summary>Whether a publisher is connected. A path that is not ready is drawn dimmed.</summary>
    public bool IsReady { get => _isReady; private set => Set(ref _isReady, value); }

    /// <summary>Whether this machine has any viewer open on this stream.</summary>
    public bool IsWatched { get => _isWatched; private set => Set(ref _isWatched, value); }

    /// <summary>
    /// The one render function. Safe to run twice: every output is written on every pass, the
    /// legs compare equal across two passes over one row, and the commands are reused by value.
    /// </summary>
    public void Apply(StreamRow row, IReadOnlyList<WatchLeg> legs)
    {
        Assert.NotNull(row, "a row renders the stream the relay reported");
        Assert.That(row.Name == Name, "a row renders the stream it was made for", Name, row.Name);
        Assert.NotNull(legs, "a row offers the legs the backend named");

        _watchedOn = row.WatchedOn;

        Detail = row.Detail;
        Tracks = row.Tracks;
        Readers = row.Readers == 1 ? "1 reader" : $"{row.Readers} readers";
        IsReady = row.IsReady;
        IsWatched = row.IsWatched;

        Reconcile.Onto(Legs, legs.Select(leg => new WatchLegViewModel
        {
            Value = leg.Value,
            Label = leg.Label,
            IsOpen = row.WatchedOn.Contains(leg.Value),
            Toggle = ToggleOf(leg.Value),
        }).ToList());

        Assert.That(Legs.Count == legs.Count, "a control per offered leg", Legs.Count, legs.Count);
        Assert.That(IsWatched == Legs.Any(leg => leg.IsOpen) || row.WatchedOn.Count > 0,
            "a watched stream has a leg to close", Name, row.WatchedOn.Count);
    }

    /// <summary>
    /// The command for one leg, made once and reused. Whether it opens or closes is read when it
    /// runs rather than captured now, so a command made on one pass still does the right thing
    /// on the next.
    /// </summary>
    private DelegateCommand ToggleOf(string transport)
    {
        if (_toggles.TryGetValue(transport, out var command))
        {
            return command;
        }

        command = new DelegateCommand(() => _ = _watch(Name, transport, _watchedOn.Contains(transport)));
        _toggles[transport] = command;
        return command;
    }
}

/// <summary>
/// One transport the backend offered as a watch leg: the value <c>StartWatch</c> takes and the
/// label a control shows. Both come off an option of the form's watch-leg field.
/// </summary>
public sealed record WatchLeg(string Value, string Label);
