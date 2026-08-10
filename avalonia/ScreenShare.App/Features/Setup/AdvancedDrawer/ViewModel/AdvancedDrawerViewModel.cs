using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Setup.AdvancedDrawer.ViewModel;

/// <summary>
/// The rest of the quality group, behind a drawer because that is where an expert expects
/// the raw knobs to be: the step above it is the answer for everyone else, and this is the
/// same group's remaining settings stated as a table.
///
/// <b>It holds no state of its own but the drawer's own open flag.</b> Which rows exist,
/// what each is called, what it means, which are greyed and why are the backend's answers,
/// arriving through <see cref="FieldGroupViewModel"/> already decided (docs/ipc-api.md,
/// "The rule"). Every write leaves through the field the reader moved.
///
/// It once carried a table of its own - an element name, five property rows, their defaults
/// and a changed count - seeded from a mockup while there was no backend behind it. All of
/// it is gone rather than kept alongside the real fields: a screen that prints a value the
/// backend never reported is a screen that lies, and the reader cannot tell which half is
/// which. What the form does not carry, this drawer no longer shows.
///
/// <b>Inputs</b>: <see cref="IsOpen"/>, a named write that <see cref="Apply"/> reads and
/// never assigns.
/// </summary>
public sealed class AdvancedDrawerViewModel : Observable
{
    private readonly FieldGroupViewModel _group;

    public AdvancedDrawerViewModel(FieldGroupViewModel group)
    {
        Assert.NotNull(group, "the drawer draws the part of a resolved group the step leaves it");

        _group = group;
        Rows = [];
        ToggleOpenCommand = new DelegateCommand(ToggleOpen);

        // Rendered off the notification rather than off a copy, the same way the step above
        // reads the same group (docs/development-principles.md, "State is written explicitly
        // and read continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private bool _isOpen = true;

    /// <summary>
    /// Whether the table is showing. Held here rather than in the view because the drawer
    /// is a piece of state the reader set, not a widget's mood.
    /// </summary>
    public bool IsOpen
    {
        get => _isOpen;
        set
        {
            if (Set(ref _isOpen, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private Icons _caretGlyph = Icons.IconChevronRight;
    private string _countLabel = "";
    private bool _hasRows;

    /// <summary>The controls this drawer draws: the group's fields the step above places nowhere.</summary>
    public ObservableCollection<FieldViewModel> Rows { get; }

    public DelegateCommand ToggleOpenCommand { get; }

    /// <summary>Open or closed, as the one glyph that says which.</summary>
    public Icons CaretGlyph { get => _caretGlyph; private set => Set(ref _caretGlyph, value); }

    /// <summary>
    /// How many settings are in here, so a closed drawer says what it is hiding. Derived
    /// from the rows rather than stored, so the header cannot claim a count the table below
    /// it does not show.
    /// </summary>
    public string CountLabel { get => _countLabel; private set => Set(ref _countLabel, value); }

    /// <summary>
    /// Whether the group left this drawer anything. False is the honest state before the
    /// first resolve and for a group whose every field the step places itself, and the
    /// drawer is not drawn at all rather than drawn empty.
    /// </summary>
    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function. Safe to run twice: the group hands back the same field view
    /// models by key, so an unchanged pass reconciles onto an equal list and no binding fires.
    /// </summary>
    public void Apply()
    {
        Reconcile.Onto(Rows, Drawn());

        CaretGlyph = IsOpen ? Icons.IconChevronDown : Icons.IconChevronRight;
        HasRows = Rows.Count > 0;
        CountLabel = Rows.Count == 1 ? "1 setting" : $"{Rows.Count} settings";

        Assert.That(HasRows == (Rows.Count > 0), "the drawer is drawn when it has rows", HasRows, Rows.Count);
        Assert.That(
            Rows.All(QualityLayout.InDrawer),
            "the drawer draws only what the step above it does not", Rows.Count);
    }

    /// <summary>Not idempotent, and the verb says so.</summary>
    private void ToggleOpen() => IsOpen = !IsOpen;

    /// <summary>
    /// The rows: every field of the group the step above places nowhere, in the order the
    /// form gave them. Chosen by <see cref="QualityLayout"/> rather than by a list of keys,
    /// so a knob the form adds shows up here without an edit.
    /// </summary>
    private IReadOnlyList<FieldViewModel> Drawn()
    {
        var drawn = new List<FieldViewModel>(_group.Fields.Count);
        foreach (var field in _group.Fields)
        {
            if (QualityLayout.InDrawer(field))
            {
                drawn.Add(field);
            }
        }

        return drawn;
    }
}
