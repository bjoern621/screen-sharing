using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.AudioStep.ViewModel;

/// <summary>
/// Audio group of the resolved form, drawn as the list it is instead of by the generic renderer.
///
/// No state of its own.
/// Which kinds exist, what each is called, which devices sit inside one, which are greyed
/// and why all arrive through <see cref="FieldGroupViewModel"/> already decided (<c>docs/ipc-api.md</c>, "The rule").
///
/// What this class owns is placement.
/// Every entry of the list carries the same four controls, so the generic renderer draws their labels, their paragraphs
/// and every option's paragraph once per entry.
/// Here the entries are rows under one set of column heads, and each control's copy is written once,
/// on the head it stands in.
///
/// The list grows and shrinks through the settings, as the contract has it:
/// the trailing row is the form's own row past the end, so picking a kind on it writes the entry,
/// and the button on a row picks the absent kind, which is what takes one off.
///
/// Outputs only, written by <see cref="Apply"/> on every pass, the branches that turn a control off included.
/// </summary>
public sealed class AudioStepViewModel : Observable
{
    private readonly FieldGroupViewModel _group;

    /// <summary>
    /// One row per entry index, kept across passes so an unchanged pass reconciles onto an equal list.
    /// </summary>
    private readonly Dictionary<int, AudioSourceRowViewModel> _rows = [];

    private string _title = "";
    private string _help = "";
    private string _summary = "";
    private bool _isResolved;
    private bool _hasRows;
    private string _liveLine = "";
    private bool _hasLiveLine;
    private FieldViewModel? _add;
    private bool _hasAdd;

    public AudioStepViewModel(FieldGroupViewModel group)
    {
        Assert.NotNull(group, "the audio step draws a group of the resolved form");

        _group = group;
        Rows = [];
        UnderList = [];

        // The group is rendered by the flow owning the draft, so a change arrives as a notification
        // and never as a copy held here (docs/development-principles.md, "State is written explicitly
        // and read continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    /// <summary>The entries the settings carry, in the form's order.</summary>
    public ObservableCollection<AudioSourceRowViewModel> Rows { get; }

    /// <summary>Every control of the group that belongs to no entry, drawn under the list.</summary>
    public ObservableCollection<FieldViewModel> UnderList { get; }

    public string Title { get => _title; private set => Set(ref _title, value); }

    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>What this step settled on, in the backend's own sentence, as the strip repeats it.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>
    /// Whether the form carries this group at all.
    /// False before the first resolve and for a backend that does not describe the step,
    /// and the card is then not drawn rather than drawn empty.
    /// </summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>
    /// Source control of the form's row past the end of the list, null where the form drew none.
    /// Picking a kind on it is the write that adds an entry,
    /// so the button carrying it says that rather than naming the absent kind it holds.
    /// </summary>
    public FieldViewModel? Add { get => _add; private set => Set(ref _add, value); }

    public bool HasAdd { get => _hasAdd; private set => Set(ref _hasAdd, value); }

    /// <summary>Whether the stream carries a source at all, which is what the column heads stand over.</summary>
    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// Which controls of the list reach a stream that is already running, empty where none does.
    /// Read off the controls and drawn over the columns: it is the control's own fact and the same on every row,
    /// so a copy of it per entry would be the repetition this layout exists to end.
    /// </summary>
    public string LiveLine { get => _liveLine; private set => Set(ref _liveLine, value); }

    public bool HasLiveLine { get => _hasLiveLine; private set => Set(ref _hasLiveLine, value); }

    public string SourceLabel => Copy.Fields.Of(AudioLayout.SourceKey).Label;

    public string SourceHelp => Copy.Fields.Of(AudioLayout.SourceKey).Help;

    public string DeviceLabel => Copy.Fields.Of(AudioLayout.DeviceKey).Label;

    public string DeviceHelp => Copy.Fields.Of(AudioLayout.DeviceKey).Help;

    public string GainLabel => Copy.Fields.Of(AudioLayout.GainKey).Label;

    public string GainHelp => Copy.Fields.Of(AudioLayout.GainKey).Help;

    public string MuteLabel => Copy.Fields.Of(AudioLayout.MuteKey).Label;

    public string MuteHelp => Copy.Fields.Of(AudioLayout.MuteKey).Help;

    public string AddLabel => Cards.AudioAdd;

    /// <summary>What a stream with nothing in the list sends, in place of the rows.</summary>
    public string EmptyLine => Cards.AudioEmpty;

    /// <summary>
    /// The one render function.
    /// Safe to run twice: rows are kept by entry index and the group hands back the same controls by key,
    /// so an unchanged pass assigns the same references and reconciles onto equal lists.
    /// </summary>
    public void Apply()
    {
        IsResolved = _group.IsResolved;
        Title = _group.Title;
        Help = _group.Help;
        Summary = _group.Summary;

        var entries = new SortedDictionary<int, Entry>();
        var under = new List<FieldViewModel>(_group.Fields.Count);
        foreach (var field in _group.Fields)
        {
            if (!AudioLayout.InRow(field))
            {
                under.Add(field);
                continue;
            }

            var entry = AudioLayout.EntryOf(field.Key);
            Assert.That(entry >= 0, "a control of the list is addressed to an entry of it", field.Key);

            if (!entries.TryGetValue(entry, out var row))
            {
                row = new Entry();
                entries[entry] = row;
            }

            row.Take(field);
        }

        // The form draws one row past the last entry the settings carry,
        // so the trailing one is what a reader grows the list by,
        // and what states the absent kind for every row above it.
        var trailing = entries.Count == 0 ? -1 : entries.Keys.Last();
        Add = trailing < 0 ? null : entries[trailing].Source;
        HasAdd = Add is not null;

        var absent = PickedOn(Add);
        var rows = new List<AudioSourceRowViewModel>(entries.Count);
        foreach (var (entry, fields) in entries)
        {
            if (entry == trailing)
            {
                continue;
            }

            var row = RowOf(entry);
            row.Apply(fields.Source, fields.Device, fields.Gain, fields.Mute, absent);
            rows.Add(row);
        }

        Reconcile.Onto(Rows, rows);
        Reconcile.Onto(UnderList, under);

        HasRows = Rows.Count > 0;
        LiveLine = LiveLineOf(Rows.Count > 0 ? Rows[0] : null);
        HasLiveLine = LiveLine.Length > 0;

        Assert.That(IsResolved || Rows.Count == 0, "a step the form did not describe draws no rows", Rows.Count);
        Assert.That(IsResolved || !HasAdd, "a step the form did not describe grows no list", HasAdd);
        Assert.That(HasRows == (Rows.Count > 0), "the row flag and the rows agree", HasRows, Rows.Count);
    }

    /// <summary>
    /// The sentence over the columns naming what reaches a running stream, off one row:
    /// every row draws the same controls, and whether one is live is the control's answer rather than the entry's.
    /// </summary>
    private static string LiveLineOf(AudioSourceRowViewModel? row)
    {
        var live = new List<string>(2);
        if (row?.Gain is { AppliesLive: true } gain)
        {
            live.Add(gain.Label);
        }

        if (row?.Mute is { AppliesLive: true } mute)
        {
            live.Add(mute.Label);
        }

        return live.Count switch
        {
            0 => "",
            1 => Cards.AudioLive(live[0]),
            _ => Cards.AudioLive(live[0], live[1]),
        };
    }

    /// <summary>Value a choice control holds, empty for a control holding none and for no control at all.</summary>
    private static string PickedOn(FieldViewModel? field)
    {
        if (field is null)
        {
            return "";
        }

        foreach (var option in field.Options)
        {
            if (option.IsSelected)
            {
                return option.Value;
            }
        }

        return "";
    }

    private AudioSourceRowViewModel RowOf(int entry)
    {
        if (_rows.TryGetValue(entry, out var row))
        {
            return row;
        }

        row = new AudioSourceRowViewModel(entry);
        _rows[entry] = row;
        return row;
    }

    /// <summary>One entry's controls while a pass sorts the group's fields into rows.</summary>
    private sealed class Entry
    {
        public FieldViewModel? Source { get; private set; }

        public FieldViewModel? Device { get; private set; }

        public FieldViewModel? Gain { get; private set; }

        public FieldViewModel? Mute { get; private set; }

        public void Take(FieldViewModel field)
        {
            switch (Copy.Fields.Template(field.Key))
            {
                case AudioLayout.SourceKey:
                    Source = field;
                    break;
                case AudioLayout.DeviceKey:
                    Device = field;
                    break;
                case AudioLayout.GainKey:
                    Gain = field;
                    break;
                case AudioLayout.MuteKey:
                    Mute = field;
                    break;
                default:
                    Assert.Never("a control of the list is one of the four a row draws", field.Key);
                    break;
            }
        }
    }
}
