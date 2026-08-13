using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Setup.AdvancedDrawer.ViewModel;

/// <summary>
/// The rest of the quality group as a table, behind a drawer: the step above is the answer for everyone else,
/// and this is where an expert reaches the raw knobs.
///
/// The open flag is the only state held here.
/// Which rows exist, what each is called and means, which are greyed and why arrive through
/// <see cref="FieldGroupViewModel"/> already decided (docs/ipc-api.md, "The rule"), and every write leaves
/// through the field the reader moved.
///
/// Inputs: <see cref="IsOpen"/>, a named write <see cref="Apply"/> reads and never assigns.
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

        // Rendered off the group's notification rather than off a copy of it, the same way the step above reads
        // the same group (docs/development-principles.md, "State is written explicitly and read continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private bool _isOpen = true;

    /// <summary>
    /// Whether the table is showing.
    /// Held here rather than in the view because an open drawer is state the reader set, not a widget's mood.
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

    /// <summary>The group's fields the step above places nowhere. Shared with it, not copies.</summary>
    public ObservableCollection<FieldViewModel> Rows { get; }

    public DelegateCommand ToggleOpenCommand { get; }

    public Icons CaretGlyph { get => _caretGlyph; private set => Set(ref _caretGlyph, value); }

    /// <summary>
    /// What a closed drawer is hiding.
    /// Derived from the rows rather than stored, so the header cannot claim a count the table does not show.
    /// </summary>
    public string CountLabel { get => _countLabel; private set => Set(ref _countLabel, value); }

    /// <summary>
    /// Whether the group left this drawer anything.
    /// False before the first resolve and for a group whose every field the step places itself, and the drawer
    /// is then not drawn at all rather than drawn empty.
    /// </summary>
    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group hands back the same field view models by key, so an unchanged pass
    /// reconciles onto an equal list and nothing notifies.
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

    /// <summary>The one departure from idempotency here, and the verb says so: two presses are two states.</summary>
    private void ToggleOpen() => IsOpen = !IsOpen;

    /// <summary>
    /// Every field of the group the step above places nowhere, in the order the form gave them.
    /// Chosen by <see cref="QualityLayout"/> rather than by a list of keys, so a knob the form adds shows up
    /// here with nothing to edit.
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
