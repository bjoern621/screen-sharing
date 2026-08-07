using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.QualityStep.ViewModel;

/// <summary>
/// The quality step: the same group of the resolved form the generic renderer would draw,
/// laid out the way this screen is designed instead.
///
/// <b>It holds no state of its own.</b> Which rate-control modes exist, what each is called,
/// what the quantizer's ends mean, which resolutions this source can be scaled to, which
/// frame rates the panel can produce and what a keyframe interval works out to are all the
/// backend's answers, arriving through <see cref="FieldGroupViewModel"/> already decided
/// (docs/ipc-api.md, "The rule"). Every write leaves through the field the reader moved.
///
/// What this class does is placement, which the contract leaves to the shell: the rate
/// control is drawn as cards because each option carries a paragraph, the quantizer sits on a
/// banded track because its scale has named zones, and everything else the group offers as a
/// select goes in the read-back row at the foot. A field the form adds to the group appears
/// there without an edit here; a field it removes stops being drawn.
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass, including the branches
/// that turn a control off. The step re-renders when the group's fields change, which is the
/// same notification the generic renderer redraws from.
/// </summary>
public sealed class QualityStepViewModel : Observable
{
    /// <summary>
    /// The two fields this layout places by name, read from the table that also tells the
    /// drawer below what it is left with. Everything else the group carries is drawn
    /// generically, so those two keys are the whole of what this screen assumes about it.
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
    private FieldViewModel? _mode;
    private FieldViewModel? _quantizer;

    public QualityStepViewModel(FieldGroupViewModel group)
    {
        Assert.NotNull(group, "the quality step draws a group of the resolved form");

        _group = group;
        Selects = [];

        // The group is rendered by the flow that owns the draft, so this step learns of a
        // change the same way any other reader does: from the notification, never by holding
        // a copy (docs/development-principles.md, "State is written explicitly and read
        // continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    /// <summary>The read-back row: every control the group offers as a select, in the form's order.</summary>
    public ObservableCollection<FieldViewModel> Selects { get; }

    public string Title { get => _title; private set => Set(ref _title, value); }

    public string Help { get => _help; private set => Set(ref _help, value); }

    /// <summary>What this step settled on, as the strip repeats it. The backend's sentence.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    /// <summary>
    /// Whether the form carries this group at all. False is the honest state before the first
    /// resolve and for a backend that does not describe the step, and the card is not drawn
    /// at all rather than drawn empty or drawn around a sentence of its own: an unreachable
    /// backend is reported once, above the column that would hold it.
    /// </summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>The rate control, drawn as cards. Null where the group does not offer it.</summary>
    public FieldViewModel? Mode { get => _mode; private set => Set(ref _mode, value); }

    /// <summary>The quantizer, drawn on the banded track. Null where the group does not offer it.</summary>
    public FieldViewModel? Quantizer { get => _quantizer; private set => Set(ref _quantizer, value); }

    public bool HasMode { get => _hasMode; private set => Set(ref _hasMode, value); }

    public bool HasQuantizer { get => _hasQuantizer; private set => Set(ref _hasQuantizer, value); }

    /// <summary>
    /// The one render function. Safe to run twice: the group hands back the same field view
    /// models by key, so an unchanged pass assigns the same references and reconciles onto an
    /// equal list, and no binding fires.
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

        Reconcile.Onto(Selects, Placed());

        Assert.That(IsResolved || Selects.Count == 0, "a step the form did not describe draws no controls", Selects.Count);
        Assert.That(HasMode == (Mode is not null), "the mode flag and the mode agree", HasMode);
    }

    /// <summary>
    /// The controls the read-back row draws: every options field the group offers, in the
    /// order the form gave them and minus the two this layout places itself.
    ///
    /// Chosen by control kind rather than by key, which is what keeps the row open: a
    /// resolution, a frame rate and a keyframe interval are three dropdowns to this code and
    /// nothing more, and a fourth one arrives without a line changing here. The fields with
    /// no options fall to the drawer below, which is the other half of
    /// <see cref="QualityLayout"/>.
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
