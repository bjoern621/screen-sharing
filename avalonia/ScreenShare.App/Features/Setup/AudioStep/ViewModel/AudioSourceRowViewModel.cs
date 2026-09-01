using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.AudioStep.ViewModel;

/// <summary>
/// One entry of the audio source list: the four controls the form drew for it, on one line.
///
/// No copy and no state of its own.
/// The controls arrive from the group's renderer already decided,
/// and every write leaves through the one the reader moved.
///
/// Outputs only, written by <see cref="Apply"/> on every pass, the branches that turn a control off included.
/// </summary>
public sealed class AudioSourceRowViewModel : Observable
{
    /// <summary>
    /// Value the absent kind is spelled by, read off the form rather than written down here:
    /// it is what the row past the end of the list holds, and picking it on an entry is what takes that entry off
    /// (<c>api/proto/screenshare/v1/settings.proto</c>, <c>AudioSource</c>).
    /// Empty until a pass has one to read.
    /// </summary>
    private string _absent = "";

    private FieldViewModel? _source;
    private FieldViewModel? _device;
    private FieldViewModel? _gain;
    private FieldViewModel? _mute;
    private bool _hasSource;
    private bool _hasDevice;
    private bool _hasGain;
    private bool _hasMute;
    private bool _canRemove;

    public AudioSourceRowViewModel(int entry)
    {
        Assert.That(entry >= 0, "a row draws an entry of the list", entry);

        Entry = entry;
        Reasons = [];
        Notes = [];
        RemoveCommand = new DelegateCommand(Remove, () => CanRemove);
    }

    /// <summary>Which entry of <c>publish.audio_sources</c> this row draws.</summary>
    public int Entry { get; }

    /// <summary>
    /// Why the controls on this row are inert, one line each, in the order the row draws them.
    /// A line under the row rather than a tip on the control: nothing opens over a greyed control,
    /// and the column head above it explains what the control does rather than why this one cannot
    /// (<c>docs/tooltips.md</c>, "An availability note is a line").
    /// </summary>
    public ObservableCollection<string> Reasons { get; }

    /// <summary>What a control on this row does beyond its own copy, for the ones the form annotates.</summary>
    public ObservableCollection<string> Notes { get; }

    /// <summary>Where this entry records from. Null where the form drew no such control.</summary>
    public FieldViewModel? Source { get => _source; private set => Set(ref _source, value); }

    /// <summary>Which device or application inside that kind.</summary>
    public FieldViewModel? Device { get => _device; private set => Set(ref _device, value); }

    public FieldViewModel? Gain { get => _gain; private set => Set(ref _gain, value); }

    public FieldViewModel? Mute { get => _mute; private set => Set(ref _mute, value); }

    public bool HasSource { get => _hasSource; private set => Set(ref _hasSource, value); }

    public bool HasDevice { get => _hasDevice; private set => Set(ref _hasDevice, value); }

    public bool HasGain { get => _hasGain; private set => Set(ref _hasGain, value); }

    public bool HasMute { get => _hasMute; private set => Set(ref _hasMute, value); }

    /// <summary>
    /// Whether this row offers the button that takes the entry off.
    /// False where the entry already names the absent kind, and where the control does not offer it:
    /// a kind the form refuses is one no press may write.
    /// </summary>
    public bool CanRemove { get => _canRemove; private set => Set(ref _canRemove, value); }

    public DelegateCommand RemoveCommand { get; }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group hands back the same controls by key,
    /// so an unchanged pass assigns the same references and fires no binding.
    /// </summary>
    /// <param name="absent">
    /// What the absent kind is called on this form, empty where no pass has read it yet.
    /// </param>
    public void Apply(
        FieldViewModel? source,
        FieldViewModel? device,
        FieldViewModel? gain,
        FieldViewModel? mute,
        string absent)
    {
        Assert.NotNull(absent, "a row is told what the absent kind is spelled by");

        _absent = absent;

        Source = source;
        Device = device;
        Gain = gain;
        Mute = mute;
        HasSource = source is not null;
        HasDevice = device is not null;
        HasGain = gain is not null;
        HasMute = mute is not null;

        Reconcile.Onto(Reasons, Sentences(field => field.HasReason ? field.Reason : ""));
        Reconcile.Onto(Notes, Sentences(field => field.HasNote ? field.Note : ""));

        CanRemove = Removal() is not null;
        RemoveCommand.Refresh();

        Assert.That(!CanRemove || HasSource, "the button that empties a row has a control to write through", Entry);
    }

    /// <summary>
    /// Entry of the source control that empties this row, null where there is none to press.
    /// The write goes through the control's own option, which is the boundary the dropdown beside it writes through,
    /// so the row decides nothing the menu would not.
    /// </summary>
    private OptionViewModel? Removal()
    {
        if (_absent.Length == 0 || Source is not { } source)
        {
            return null;
        }

        foreach (var option in source.Options)
        {
            if (option.Value == _absent)
            {
                return option.IsSelected || !option.IsEnabled ? null : option;
            }
        }

        return null;
    }

    private void Remove() => Removal()?.Choose.Execute(null);

    /// <summary>
    /// One sentence per control that has one, in the order the row draws them.
    /// A sentence two controls state alike is drawn once: a row whose kind is unset greys its device, its level
    /// and its mute on the one fact, and three copies of it read as three faults.
    /// </summary>
    private IReadOnlyList<string> Sentences(Func<FieldViewModel, string> of)
    {
        var lines = new List<string>(4);
        foreach (var field in new[] { Source, Device, Gain, Mute })
        {
            if (field is null)
            {
                continue;
            }

            var line = of(field);
            if (line.Length > 0 && !lines.Contains(line))
            {
                lines.Add(line);
            }
        }

        return lines;
    }
}
