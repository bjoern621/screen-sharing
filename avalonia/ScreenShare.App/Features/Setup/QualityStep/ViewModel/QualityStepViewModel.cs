using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.QualityStep.ViewModel;

/// <summary>
/// Quality group of the resolved form, laid out the way this screen is designed instead of by the generic renderer.
///
/// No state of its own.
/// Which rate-control modes exist, what each is called, which end of the quantizer scale keeps more detail,
/// which resolutions this source scales to, which frame rates the panel produces
/// and what a keyframe interval works out to all arrive through <see cref="FieldGroupViewModel"/> already decided
/// (<c>docs/ipc-api.md</c>, "The rule").
/// Every write leaves through the field the reader moved.
///
/// What this class owns is placement, which the contract leaves to the shell: cards for the rate control,
/// each option carrying a paragraph; the banded track for the quantizer, its scale having named zones;
/// every other select in the read-back row at the foot.
/// A field the form adds to the group is drawn without an edit here, and one it removes stops being drawn.
///
/// Outputs only, written by <see cref="Apply"/> on every pass, the branches that turn a control off included.
/// </summary>
public sealed class QualityStepViewModel : Observable
{
    /// <summary>
    /// The two keys this layout places by name, off the table the card below reads as its complement.
    /// The whole of what this screen assumes about the group.
    /// </summary>
    private const string ModeKey = QualityLayout.ModeKey;
    private const string QuantizerKey = QualityLayout.QuantizerKey;

    private readonly FieldGroupViewModel _group;

    /// <summary>
    /// Mode control whose offered entries this step is following, null while the group carries none.
    /// Held so a later form handing back another control under the same key moves the subscription,
    /// instead of adding a second one.
    /// </summary>
    private FieldViewModel? _listening;

    private string _title = "";
    private string _help = "";
    private string _summary = "";
    private bool _isResolved;
    private bool _hasMode;
    private bool _hasQuantizer;
    private int _modeColumns = 1;
    private FieldViewModel? _mode;
    private FieldViewModel? _quantizer;
    private string _quantizerFloorLabel = "";
    private string _quantizerBandLabel = "";
    private string _quantizerCeilingLabel = "";

    public QualityStepViewModel(FieldGroupViewModel group)
    {
        Assert.NotNull(group, "the quality step draws a group of the resolved form");

        _group = group;
        Selects = [];

        // The group is rendered by the flow owning the draft, so a change arrives as a notification
        // and never as a copy held here (docs/development-principles.md, "State is written explicitly
        // and read continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    /// <summary>
    /// The disclosure the whole step sits behind: every quality control is covered by a preset,
    /// so the floor of this step is empty and the fold carries it all, the Advanced card included
    /// (<c>Setup/View/SetupView.axaml</c> gates that card on it).
    /// </summary>
    public FoldViewModel Fold { get; } = new();

    /// <summary>Read-back row: every control the group offers as a select, in the form's order.</summary>
    public ObservableCollection<FieldViewModel> Selects { get; }

    public string Title { get => _title; private set => Set(ref _title, value); }

    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>What this step settled on, in the backend's own sentence, as the strip repeats it.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>
    /// Whether the form carries this group at all.
    /// False before the first resolve and for a backend that does not describe the step,
    /// and the card is then not drawn rather than drawn empty: an unreachable backend is reported once,
    /// above the column.
    /// </summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>Rate control, drawn as cards. Null where the group does not offer it.</summary>
    public FieldViewModel? Mode { get => _mode; private set => Set(ref _mode, value); }

    /// <summary>Quantizer, drawn on the banded track. Null where the group does not offer it.</summary>
    public FieldViewModel? Quantizer { get => _quantizer; private set => Set(ref _quantizer, value); }

    public bool HasMode { get => _hasMode; private set => Set(ref _hasMode, value); }

    public bool HasQuantizer { get => _hasQuantizer; private set => Set(ref _hasQuantizer, value); }

    /// <summary>
    /// Three labels under the banded track, each naming the number on this control's scale it stands over.
    /// The scale arrives with the field and differs per codec and engine,
    /// so the numbers are read off the range the slider is bound to rather than written into the markup
    /// (<see cref="QualityLayout.QuantizerBandStart"/>).
    /// </summary>
    public string QuantizerFloorLabel { get => _quantizerFloorLabel; private set => Set(ref _quantizerFloorLabel, value); }

    public string QuantizerBandLabel { get => _quantizerBandLabel; private set => Set(ref _quantizerBandLabel, value); }

    public string QuantizerCeilingLabel { get => _quantizerCeilingLabel; private set => Set(ref _quantizerCeilingLabel, value); }

    /// <summary>
    /// How many rate-control cards sit across the step, for the entries the grid is about to draw.
    /// A shape rather than a control: the panel divides the same entries into rows from it
    /// (<see cref="QualityLayout.CardColumns"/>).
    /// Counted off <see cref="FieldViewModel.Offered"/> and not <see cref="FieldViewModel.Options"/>,
    /// the two parting company as soon as the availability pass refuses an entry:
    /// a count including the refused ones opens a column the last row has no card for,
    /// the empty row <c>CardColumns</c> asserts against.
    /// The refused cards take the same shape under the disclosure, so a grid over it is a grid the reveal leaves put.
    /// </summary>
    public int ModeColumns { get => _modeColumns; private set => Set(ref _modeColumns, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group hands back the same field view models by key,
    /// so an unchanged pass assigns the same references, reconciles onto an equal list and fires no binding.
    /// </summary>
    public void Apply()
    {
        IsResolved = _group.IsResolved;
        Title = _group.Title;
        Help = _group.Help;
        Summary = _group.Summary;

        Mode = _group.Visible(ModeKey);
        Quantizer = _group.Visible(QuantizerKey);
        HasMode = Mode is not null;
        HasQuantizer = Quantizer is not null;
        Listen(Mode);
        ModeColumns = QualityLayout.CardColumns(Mode?.Offered.Count ?? 0);
        ApplyQuantizerLabels();

        Reconcile.Onto(Selects, Placed());

        // The whole group: mode, quantizer, selects and the Advanced card's rows all sit behind the fold.
        Fold.Apply(_group.Fields.Count);

        Assert.That(IsResolved || Selects.Count == 0, "a step the form did not describe draws no controls", Selects.Count);
        Assert.That(HasMode == (Mode is not null), "the mode flag and the mode agree", HasMode);
    }

    /// <summary>
    /// Follows the mode control's offered entries.
    /// The two subscriptions the constructor takes do not cover this one path:
    /// a pass refusing a mode rebuilds <see cref="FieldViewModel.Offered"/> without the group's properties
    /// or its field list moving, so nothing would call the render function
    /// and the column count would be left over from the list before the refusal.
    /// </summary>
    private void Listen(FieldViewModel? field)
    {
        if (ReferenceEquals(_listening, field))
        {
            return;
        }

        if (_listening is not null)
        {
            ((INotifyCollectionChanged)_listening.Offered).CollectionChanged -= OnOfferedChanged;
        }

        _listening = field;

        if (field is not null)
        {
            ((INotifyCollectionChanged)field.Offered).CollectionChanged += OnOfferedChanged;
        }
    }

    private void OnOfferedChanged(object? sender, NotifyCollectionChangedEventArgs e) => Apply();

    /// <summary>
    /// Labels under the track, off the range the control was offered on.
    /// A group carrying no quantizer clears them,
    /// so nothing left over from the codec before is drawn beside the next one.
    /// </summary>
    private void ApplyQuantizerLabels()
    {
        if (Quantizer is not { } q)
        {
            QuantizerFloorLabel = "";
            QuantizerBandLabel = "";
            QuantizerCeilingLabel = "";
            return;
        }

        var from = QualityLayout.QuantizerAt(q.Minimum, q.Maximum, QualityLayout.QuantizerBandStart);
        var to = QualityLayout.QuantizerAt(q.Minimum, q.Maximum, QualityLayout.QuantizerBandEnd);

        QuantizerFloorLabel = Cards.QuantizerFloor((int)q.Minimum);
        QuantizerBandLabel = Cards.QuantizerBand(from, to);
        QuantizerCeilingLabel = Cards.QuantizerCeiling((int)q.Maximum);
    }

    /// <summary>
    /// Read-back row's controls: every options field the group offers, in the form's order,
    /// minus the two this layout places itself.
    /// Chosen by control kind rather than by key, so a resolution, a frame rate
    /// and a keyframe interval are three dropdowns to this code and a fourth arrives without a line changing.
    /// A field with no options falls to the card below, the other half of <see cref="QualityLayout"/>.
    /// </summary>
    private IReadOnlyList<FieldViewModel> Placed()
    {
        var placed = new List<FieldViewModel>(_group.Fields.Count);
        foreach (var field in _group.Fields)
        {
            if (QualityLayout.InReadbackRow(field))
            {
                placed.Add(field);
            }
        }

        return placed;
    }
}
