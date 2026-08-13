using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.QualityStep.ViewModel;

/// <summary>
/// The quality group of the resolved form, laid out the way this screen is designed instead of by the generic
/// renderer.
///
/// No state of its own.
/// Which rate-control modes exist, what each is called, which end of the quantizer scale keeps more detail,
/// which resolutions this source scales to, which frame rates the panel produces and what a keyframe interval
/// works out to all arrive through <see cref="FieldGroupViewModel"/> already decided (docs/ipc-api.md, "The
/// rule").
/// Every write leaves through the field the reader moved.
///
/// What is this class's is placement, which the contract leaves to the shell: cards for the rate control
/// because each option carries a paragraph, the banded track for the quantizer because its scale has named
/// zones, and every other select in the read-back row at the foot.
/// A field the form adds to the group is drawn without an edit here, and one it removes stops being drawn.
///
/// Outputs only, written by <see cref="Apply"/> on every pass, including the branches that turn a control off.
/// </summary>
public sealed class QualityStepViewModel : Observable
{
    /// <summary>
    /// The two keys this layout places by name, off the table the drawer below reads as its complement.
    /// The whole of what this screen assumes about the group.
    /// </summary>
    private const string ModeKey = QualityLayout.ModeKey;
    private const string QuantizerKey = QualityLayout.QuantizerKey;

    private readonly FieldGroupViewModel _group;

    private string _title = "";
    private string _help = "";
    private string _summary = "";
    private bool _isResolved;
    private bool _hasMode;
    private bool _hasQuantizer;
    private int _modeColumns = 1;
    private FieldViewModel? _mode;
    private FieldViewModel? _quantizer;

    public QualityStepViewModel(FieldGroupViewModel group)
    {
        Assert.NotNull(group, "the quality step draws a group of the resolved form");

        _group = group;
        Selects = [];

        // The group is rendered by the flow that owns the draft, so a change arrives as a notification and
        // never as a copy held here (docs/development-principles.md, "State is written explicitly and read
        // continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    /// <summary>The read-back row: every control the group offers as a select, in the form's order.</summary>
    public ObservableCollection<FieldViewModel> Selects { get; }

    public string Title { get => _title; private set => Set(ref _title, value); }

    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>What this step settled on, in the backend's own sentence, as the strip repeats it.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>
    /// Whether the form carries this group at all.
    /// False before the first resolve and for a backend that does not describe the step, and the card is then
    /// not drawn rather than drawn empty: an unreachable backend is reported once, above the column.
    /// </summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>The rate control, drawn as cards. Null where the group does not offer it.</summary>
    public FieldViewModel? Mode { get => _mode; private set => Set(ref _mode, value); }

    /// <summary>The quantizer, drawn on the banded track. Null where the group does not offer it.</summary>
    public FieldViewModel? Quantizer { get => _quantizer; private set => Set(ref _quantizer, value); }

    public bool HasMode { get => _hasMode; private set => Set(ref _hasMode, value); }

    public bool HasQuantizer { get => _hasQuantizer; private set => Set(ref _hasQuantizer, value); }

    /// <summary>
    /// How many rate-control cards sit across the step, for the mode count this form offers.
    /// A shape rather than a control: the panel divides the same options into rows from it
    /// (<see cref="QualityLayout.CardColumns"/>).
    /// </summary>
    public int ModeColumns { get => _modeColumns; private set => Set(ref _modeColumns, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group hands back the same field view models by key, so an unchanged pass
    /// assigns the same references, reconciles onto an equal list and fires no binding.
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
        ModeColumns = QualityLayout.CardColumns(Mode?.Options.Count ?? 0);

        Reconcile.Onto(Selects, Placed());

        Assert.That(IsResolved || Selects.Count == 0, "a step the form did not describe draws no controls", Selects.Count);
        Assert.That(HasMode == (Mode is not null), "the mode flag and the mode agree", HasMode);
    }

    /// <summary>
    /// The read-back row's controls: every options field the group offers, in the form's order, minus the two
    /// this layout places itself.
    /// Chosen by control kind rather than by key, so a resolution, a frame rate and a keyframe interval are
    /// three dropdowns to this code and a fourth arrives without a line changing.
    /// A field with no options falls to the drawer, the other half of <see cref="QualityLayout"/>.
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
