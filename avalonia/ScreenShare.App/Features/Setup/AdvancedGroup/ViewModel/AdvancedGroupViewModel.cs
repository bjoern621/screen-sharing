using System.Collections.ObjectModel;
using System.Collections.Specialized;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.AdvancedGroup.ViewModel;

/// <summary>
/// Rest of the quality group: the step above is the answer for everyone else,
/// and this is where an expert reaches the raw knobs.
///
/// Nothing is held here.
/// Which rows exist, what each is called and means, which are greyed
/// and why arrive through <see cref="FieldGroupViewModel"/> already decided (<c>docs/ipc-api.md</c>, "The rule"),
/// and every write leaves through the field the reader moved.
/// </summary>
public sealed class AdvancedGroupViewModel : Observable
{
    private readonly FieldGroupViewModel _group;

    public AdvancedGroupViewModel(FieldGroupViewModel group)
    {
        Assert.NotNull(group, "the card draws the part of a resolved group the step leaves it");

        _group = group;
        Rows = [];

        // Rendered off the group's notification rather than off a copy of it,
        // the same way the step above reads the same group (docs/development-principles.md,
        // "State is written explicitly and read continuously").
        _group.PropertyChanged += (_, _) => Apply();
        ((INotifyCollectionChanged)_group.Fields).CollectionChanged += (_, _) => Apply();

        Apply();
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _hasRows;

    /// <summary>Group's fields the step above places nowhere. Shared with it, not copies.</summary>
    public ObservableCollection<FieldViewModel> Rows { get; }

    /// <summary>
    /// Card's own heading, from the one place the on-screen words live (<c>avalonia/README.md</c>, "Layout").
    /// A fixed word: what the rows say is the form's answer, what the card is called is not.
    /// </summary>
    public static string Title => Cards.AdvancedTitle;

    /// <summary>
    /// Whether the group left this card anything.
    /// False before the first resolve and for a group whose every field the step places itself,
    /// the card then not being drawn at all rather than drawn empty.
    /// </summary>
    public bool HasRows { get => _hasRows; private set => Set(ref _hasRows, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: the group hands back the same field view models by key,
    /// so an unchanged pass reconciles onto an equal list and nothing notifies.
    /// </summary>
    public void Apply()
    {
        Reconcile.Onto(Rows, Drawn());

        HasRows = Rows.Count > 0;

        Assert.That(HasRows == (Rows.Count > 0), "the card is drawn when it has rows", HasRows, Rows.Count);
        Assert.That(
            Rows.All(QualityLayout.BelowStep),
            "the card draws only what the step above it does not", Rows.Count);
    }

    /// <summary>
    /// Every field of the group the step above places nowhere, in the order the form gave them.
    /// Chosen by <see cref="QualityLayout"/> rather than by a list of keys,
    /// so a knob the form adds shows up here with nothing to edit.
    /// </summary>
    private IReadOnlyList<FieldViewModel> Drawn()
    {
        var drawn = new List<FieldViewModel>(_group.Fields.Count);
        foreach (var field in _group.Fields)
        {
            if (QualityLayout.BelowStep(field))
            {
                drawn.Add(field);
            }
        }

        return drawn;
    }
}
